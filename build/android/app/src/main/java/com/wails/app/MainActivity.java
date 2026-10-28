package com.wails.app;

import android.annotation.SuppressLint;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.res.Configuration;
import android.database.Cursor;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.Uri;
import android.os.BatteryManager;
import android.os.Build;
import android.os.Bundle;
import android.os.PowerManager;
import android.content.pm.PackageManager;
import android.graphics.Bitmap;
import android.graphics.Color;
import android.graphics.BitmapFactory;
import android.provider.DocumentsContract;
import android.provider.MediaStore;
import android.provider.OpenableColumns;
import android.util.Base64;
import android.util.Log;
import android.webkit.MimeTypeMap;
import android.view.View;
import android.graphics.Insets;
import android.view.WindowInsets;
import android.webkit.JavascriptInterface;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Toast;

import androidx.activity.OnBackPressedCallback;
import androidx.annotation.Nullable;
import androidx.appcompat.app.AppCompatActivity;
import androidx.core.content.FileProvider;
import androidx.webkit.WebViewAssetLoader;

import org.json.JSONObject;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * MainActivity hosts the WebView and manages the Wails application lifecycle.
 * It uses WebViewAssetLoader to serve assets from the Go library without
 * requiring a network server.
 */
public class MainActivity extends AppCompatActivity {
    private static final String TAG = "WailsActivity";
    private static final boolean DEBUG = BuildConfig.DEBUG;
    private static final String WAILS_SCHEME = "https";
    private static final String WAILS_HOST = "wails.localhost";
    private static final int FILE_PICKER_REQUEST = 7001;

    private TerminalInputWebView webView;
    private WailsBridge bridge;
    // Battery: system-event receivers are registered only while the activity is
    // in the foreground (onStart) and torn down in onStop, so background battery/
    // network/screen broadcasts don't wake the app.
    private boolean systemReceiversRegistered = false;
    private WebViewAssetLoader assetLoader;

    // The Go-side dialog ID of the in-flight file picker (-1 when idle)
    private int pendingFilePickerCallbackID = -1;
    private static final int PHOTO_CAPTURE_REQUEST = 7002;
    private static final int VIDEO_CAPTURE_REQUEST = 7003;
    private static final int CAMERA_PERMISSION_REQUEST = 7010;
    private static final int SAVE_FILE_REQUEST = 7004;
    private static final int SAVE_FOLDER_REQUEST = 7005;
    private static final int PICK_UPLOAD_FOLDER_REQUEST = 7006;
    // Downloads finish in a queue, so several can ask to be saved while the
    // save dialog for the first is still open.
    private final ArrayDeque<String> pendingSavePaths = new ArrayDeque<>();
    private final ArrayDeque<String> pendingSaveFolderPaths = new ArrayDeque<>();
    private File pendingCaptureFile;
    private boolean pendingCaptureIsVideo;

    // System-event sources (battery/power, screen lock, network). Registered in
    // onCreate, torn down in onDestroy. Each forwards a "system:*" event to JS
    // via the bridge.
    private BroadcastReceiver batteryReceiver;
    private BroadcastReceiver screenReceiver;
    private BroadcastReceiver powerSaveReceiver;
    private ConnectivityManager connectivityManager;
    private ConnectivityManager.NetworkCallback networkCallback;

    // Enabled only while the page has a step to back out of. At the root, back
    // falls through to the callback that sends the task to the background.
    private OnBackPressedCallback inAppBackNavigation;

    // Re-applied on every page load, because a reload clears the page's styles.
    private String safeAreaScript;

    private static MainActivity newestInstance;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        newestInstance = this;
        setContentView(R.layout.activity_main);

        // Initialize the native Go library
        bridge = new WailsBridge(this);
        // A recreated activity and a relaunch from recents both get back the
        // intent that first opened the app, whose link was already handled.
        boolean openedByNewDeepLink = savedInstanceState == null
                && (getIntent().getFlags() & Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY) == 0
                && bridge.setPendingDeepLink(getIntent());
        bridge.initialize();
        // Go takes a cold launch's link when the app starts. When Go already
        // runs, this activity replaces a destroyed one and the app has to be told.
        if (openedByNewDeepLink) bridge.emitEvent("android:deeplink", "{}");

        // Set up WebView
        setupWebView();
        registerBackNavigation();

        // Load the application
        loadApplication();
    }

    @SuppressLint("SetJavaScriptEnabled")
    private void setupWebView() {
        webView = findViewById(R.id.webview);
        bridge.setWebView(webView);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.VANILLA_ICE_CREAM) {
            resizeForKeyboard(findViewById(R.id.main_container));
        }

        // Configure WebView settings
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(false);
        settings.setMediaPlaybackRequiresUserGesture(false);
        // overscroll-behavior on html covers scrollers inside the page, but the
        // stretch at the page's own ends is drawn by the WebView view itself.
        webView.setOverScrollMode(View.OVER_SCROLL_NEVER);
        // Transparent until the page paints, so the window's light or dark
        // background shows through rather than the WebView's default white.
        webView.setBackgroundColor(Color.TRANSPARENT);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);

        // Enable debugging in debug builds
        if (DEBUG) {
            WebView.setWebContentsDebuggingEnabled(true);
        }

        // Set up asset loader for serving local assets
        assetLoader = new WebViewAssetLoader.Builder()
                .setDomain(WAILS_HOST)
                .addPathHandler("/", new WailsPathHandler(bridge))
                .build();

        // Set up WebView client to intercept requests
        webView.setWebViewClient(new WebViewClient() {
            @Nullable
            @Override
            public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                // Handle wails.localhost requests
                if (request.getUrl().getHost() != null &&
                        request.getUrl().getHost().equals(WAILS_HOST)) {

                    // For wails API calls (capabilities, flags, etc.) pass the
                    // full URL including the query string, because
                    // WebViewAssetLoader.PathHandler strips query params
                    String path = request.getUrl().getPath();
                    // Runtime calls go through WailsJSBridge, which answers only
                    // the app's own document. Served here, a GET from any frame
                    // or page could run a bound method.
                    if ("/wails/runtime".equals(path)) {
                        return new WebResourceResponse("text/plain", "UTF-8", 403, "Forbidden",
                                new java.util.HashMap<>(), new java.io.ByteArrayInputStream(new byte[0]));
                    }
                    if (path != null && path.startsWith("/wails/")) {
                        String fullPath = path;
                        String query = request.getUrl().getQuery();
                        if (query != null && !query.isEmpty()) {
                            fullPath = path + "?" + query;
                        }
                        if (DEBUG) Log.d(TAG, "Wails API call: " + path);

                        if (isFromOtherOrigin(request)) {
                            return new WebResourceResponse(
                                "application/json",
                                "UTF-8",
                                403,
                                "Forbidden",
                                new java.util.HashMap<>(),
                                new java.io.ByteArrayInputStream("{}".getBytes())
                            );
                        }

                        byte[] data = bridge.serveAsset(fullPath, request.getMethod(), "{}");
                        if (data != null && data.length > 0) {
                            java.io.InputStream inputStream = new java.io.ByteArrayInputStream(data);
                            java.util.Map<String, String> headers = new java.util.HashMap<>();
                            headers.put("Cache-Control", "no-cache");
                            headers.put("Content-Type", "application/json");

                            // Go wraps a failed runtime call in {"ok":false,...}, and
                            // the fetch fallback only rejects on a non-2xx status.
                            int status = 200;
                            String reason = "OK";
                            if (data.length > 12) {
                                String prefix = new String(data, 0, Math.min(data.length, 20),
                                        java.nio.charset.StandardCharsets.UTF_8);
                                if (prefix.contains("\"ok\":false")) {
                                    status = 500;
                                    reason = "Internal Error";
                                }
                            }

                            return new WebResourceResponse(
                                "application/json",
                                "UTF-8",
                                status,
                                reason,
                                headers,
                                inputStream
                            );
                        }
                        // Return error response if data is null
                        return new WebResourceResponse(
                            "application/json",
                            "UTF-8",
                            500,
                            "Internal Error",
                            new java.util.HashMap<>(),
                            new java.io.ByteArrayInputStream("{}".getBytes())
                        );
                    }

                    // Stream captured photos/videos from the cache with HTTP Range
                    // support so <video> can seek/stream a clip of any length.
                    if (path != null && path.startsWith("/__capture__/")) {
                        return serveCaptureFile(path.substring("/__capture__/".length()), request);
                    }

                    // For regular assets, use the asset loader
                    return assetLoader.shouldInterceptRequest(request.getUrl());
                }

                return super.shouldInterceptRequest(view, request);
            }

            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                return request.isForMainFrame() && keepOutOfApp(request.getUrl());
            }

            // Called instead of the request variant below API 24.
            @SuppressWarnings("deprecation")
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                return keepOutOfApp(Uri.parse(url));
            }

            @Override
            public void onPageStarted(WebView view, String url, Bitmap favicon) {
                super.onPageStarted(view, url, favicon);
                // A reload starts the page with nothing open and it reports only
                // changes, so a stale claim on the button would swallow every back.
                if (inAppBackNavigation != null) inAppBackNavigation.setEnabled(false);
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                super.onPageFinished(view, url);
                if (DEBUG) Log.d(TAG, "Page loaded: " + url);
                bridge.onPageFinished(url);
                if (safeAreaScript != null) webView.evaluateJavascript(safeAreaScript, null);
                // Now that JS listeners are mounted, push a snapshot of the
                // current battery / network / theme so the UI starts populated.
                emitSystemSnapshot();
            }
        });

        if (!WailsJSBridge.install(webView, bridge, WAILS_SCHEME + "://" + WAILS_HOST)) {
            Log.e(TAG, "This WebView cannot limit the Go bridge to the app's own page, so the page gets no bridge");
        }
        webView.addJavascriptInterface(new AndroidBackBridge(), "androidBack");
        webView.addJavascriptInterface(new TerminalInputBridge(), "androidTerminalInput");
    }

    private static boolean isFromOtherOrigin(WebResourceRequest request) {
        for (java.util.Map.Entry<String, String> header : request.getRequestHeaders().entrySet()) {
            if (header.getKey().equalsIgnoreCase("Origin")) {
                return !header.getValue().equals(WAILS_SCHEME + "://" + WAILS_HOST);
            }
        }
        return false;
    }

    // The app's WebView shows only the app. A link to anywhere else opens in the
    // system browser when it is a web or mail link, and is dropped otherwise.
    private boolean keepOutOfApp(Uri url) {
        String scheme = url.getScheme();
        if (WAILS_SCHEME.equals(scheme) && WAILS_HOST.equals(url.getHost())) {
            return false;
        }
        if ("https".equals(scheme) || "http".equals(scheme) || "mailto".equals(scheme)) {
            bridge.openURL(url.toString());
        } else {
            Log.w(TAG, "Dropped a navigation to a " + scheme + " link");
        }
        return true;
    }

    private void loadApplication() {
        String url = WAILS_SCHEME + "://" + WAILS_HOST + "/";
        if (DEBUG) Log.d(TAG, "Loading URL: " + url);
        webView.loadUrl(url);
    }

    /**
     * Launch the system camera to capture a photo (video=false) or a video
     * (video=true). The capture is written to a FileProvider URI in the cache and
     * the result is delivered to JS as a "common:capture" event.
     */
    public void launchCameraCapture(boolean video) {
        if (checkSelfPermission("android.permission.CAMERA") != PackageManager.PERMISSION_GRANTED) {
            pendingCaptureIsVideo = video;
            requestPermissions(new String[]{"android.permission.CAMERA"}, CAMERA_PERMISSION_REQUEST);
            return;
        }
        try {
            File dir = new File(getCacheDir(), "captures");
            if (!dir.exists()) dir.mkdirs();
            pendingCaptureFile = new File(dir, "capture_" + System.currentTimeMillis() + (video ? ".mp4" : ".jpg"));
            pendingCaptureIsVideo = video;
            Uri uri = FileProvider.getUriForFile(this, getPackageName() + ".fileprovider", pendingCaptureFile);
            Intent intent = new Intent(video ? MediaStore.ACTION_VIDEO_CAPTURE : MediaStore.ACTION_IMAGE_CAPTURE);
            intent.putExtra(MediaStore.EXTRA_OUTPUT, uri);
            intent.addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
            // Don't pre-check with resolveActivity(): Android 11+ package visibility
            // hides other apps' intents unless declared in <queries>, so it can
            // return null even when a camera app exists. Just launch and handle a miss.
            startActivityForResult(intent, video ? VIDEO_CAPTURE_REQUEST : PHOTO_CAPTURE_REQUEST);
        } catch (android.content.ActivityNotFoundException e) {
            bridge.emitEvent("common:capture", "{\"error\":\"no camera app available\"}");
        } catch (Exception e) {
            Log.e(TAG, "launchCameraCapture failed", e);
            bridge.emitEvent("common:capture", "{\"error\":\"capture failed\"}");
        }
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode == CAMERA_PERMISSION_REQUEST) {
            if (grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED) {
                launchCameraCapture(pendingCaptureIsVideo);
            } else {
                bridge.emitEvent("common:capture", "{\"error\":\"camera permission denied\"}");
            }
            return;
        }
        if (bridge != null) {
            bridge.onRequestPermissionsResult(requestCode, grantResults);
        }
    }

    private void handleCaptureResult(int resultCode, @Nullable Intent data) {
        File file = pendingCaptureFile;
        final boolean video = pendingCaptureIsVideo;
        pendingCaptureFile = null;
        if (resultCode != RESULT_OK) {
            bridge.emitEvent("common:capture", "{\"cancelled\":true}");
            return;
        }
        // Some camera apps (commonly for video) ignore EXTRA_OUTPUT and instead
        // return a content URI in the result data; copy that into our cache.
        if ((file == null || !file.exists() || file.length() == 0)
                && data != null && data.getData() != null) {
            String copied = copyUriToCache(data.getData());
            if (copied != null) file = new File(copied);
        }
        final File f = file;
        if (f == null || !f.exists() || f.length() == 0) {
            bridge.emitEvent("common:capture", "{\"cancelled\":true}");
            return;
        }
        new Thread(() -> {
            try {
                JSONObject o = new JSONObject();
                o.put("type", video ? "video" : "photo");
                o.put("path", f.getAbsolutePath());
                o.put("size", f.length());
                if (!video) {
                    String thumb = makePhotoThumbnail(f);
                    if (thumb != null) o.put("thumb", thumb);
                }
                // Stream URL works for both: <video>/<img> load it from the cache
                // via shouldInterceptRequest (Range-enabled), no size limit.
                o.put("streamUrl", captureStreamUrl(f));
                bridge.emitEvent("common:capture", o.toString());
            } catch (Exception e) {
                Log.e(TAG, "handleCaptureResult failed", e);
                bridge.emitEvent("common:capture", "{\"error\":\"result processing failed\"}");
            }
        }).start();
    }

    /** Downscale a captured photo into a base64 JPEG data URL for display in the webview. */
    @Nullable
    private String makePhotoThumbnail(File file) {
        try {
            BitmapFactory.Options bounds = new BitmapFactory.Options();
            bounds.inJustDecodeBounds = true;
            BitmapFactory.decodeFile(file.getAbsolutePath(), bounds);
            int sample = 1;
            while (Math.max(bounds.outWidth, bounds.outHeight) / sample > 640) sample *= 2;
            BitmapFactory.Options opts = new BitmapFactory.Options();
            opts.inSampleSize = sample;
            Bitmap bmp = BitmapFactory.decodeFile(file.getAbsolutePath(), opts);
            if (bmp == null) return null;
            ByteArrayOutputStream baos = new ByteArrayOutputStream();
            bmp.compress(Bitmap.CompressFormat.JPEG, 70, baos);
            bmp.recycle();
            return "data:image/jpeg;base64," + Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP);
        } catch (Exception e) {
            return null;
        }
    }

    /**
     * Build a same-origin URL the webview can stream a capture from. Served by
     * serveCaptureFile (via shouldInterceptRequest); the path is relative to the
     * cache dir so both camera files (captures/) and copied content URIs
     * (wails-picker/) resolve.
     */
    private String captureStreamUrl(File file) {
        String base = getCacheDir().getAbsolutePath() + File.separator;
        String abs = file.getAbsolutePath();
        String rel = abs.startsWith(base) ? abs.substring(base.length()) : file.getName();
        return "/__capture__/" + Uri.encode(rel, "/");
    }

    /**
     * Serve a captured file (under the app cache) to the webview with HTTP Range
     * support, so &lt;video&gt; can stream and seek a clip of any length without
     * inlining it as a data URL.
     */
    private WebResourceResponse serveCaptureFile(String relPath, WebResourceRequest request) {
        try {
            File cache = getCacheDir();
            File file = new File(cache, Uri.decode(relPath));
            // Path-traversal guard: only ever serve files under the cache dir.
            if (!file.getCanonicalPath().startsWith(cache.getCanonicalPath() + File.separator)
                    || !file.exists() || !file.isFile()) {
                return new WebResourceResponse("text/plain", "UTF-8", 404, "Not Found",
                        new java.util.HashMap<>(), new java.io.ByteArrayInputStream(new byte[0]));
            }
            String name = file.getName().toLowerCase();
            String mime = name.endsWith(".mp4") ? "video/mp4"
                    : name.endsWith(".mov") ? "video/quicktime"
                    : name.endsWith(".jpg") || name.endsWith(".jpeg") ? "image/jpeg"
                    : name.endsWith(".png") ? "image/png" : "application/octet-stream";
            long length = file.length();
            java.util.Map<String, String> reqHeaders = request.getRequestHeaders();
            String range = reqHeaders != null ? reqHeaders.get("Range") : null;
            if (range == null && reqHeaders != null) range = reqHeaders.get("range");

            java.util.Map<String, String> headers = new java.util.HashMap<>();
            headers.put("Accept-Ranges", "bytes");
            headers.put("Cache-Control", "no-store");

            if (range != null && range.startsWith("bytes=")) {
                long start = 0, end = length - 1;
                String spec = range.substring(6).trim();
                int dash = spec.indexOf('-');
                if (dash >= 0) {
                    try {
                        if (dash > 0) start = Long.parseLong(spec.substring(0, dash).trim());
                        String e = spec.substring(dash + 1).trim();
                        if (!e.isEmpty()) end = Long.parseLong(e);
                    } catch (NumberFormatException ignored) { }
                }
                if (start < 0) start = 0;
                if (end >= length) end = length - 1;
                if (start > end) { start = 0; end = length - 1; }
                long count = end - start + 1;
                java.io.InputStream in = new java.io.FileInputStream(file);
                long toSkip = start;
                while (toSkip > 0) {
                    long s = in.skip(toSkip);
                    if (s <= 0) break;
                    toSkip -= s;
                }
                headers.put("Content-Range", "bytes " + start + "-" + end + "/" + length);
                headers.put("Content-Length", String.valueOf(count));
                return new WebResourceResponse(mime, null, 206, "Partial Content",
                        headers, new LimitedInputStream(in, count));
            }
            headers.put("Content-Length", String.valueOf(length));
            return new WebResourceResponse(mime, null, 200, "OK", headers,
                    new java.io.FileInputStream(file));
        } catch (Exception e) {
            Log.e(TAG, "serveCaptureFile failed", e);
            return new WebResourceResponse("text/plain", "UTF-8", 500, "Error",
                    new java.util.HashMap<>(), new java.io.ByteArrayInputStream(new byte[0]));
        }
    }

    /** Wraps a stream to yield at most a fixed number of bytes (for Range responses). */
    private static final class LimitedInputStream extends java.io.FilterInputStream {
        private long remaining;
        LimitedInputStream(java.io.InputStream in, long limit) {
            super(in);
            this.remaining = limit;
        }
        @Override public int read() throws java.io.IOException {
            if (remaining <= 0) return -1;
            int b = super.read();
            if (b >= 0) remaining--;
            return b;
        }
        @Override public int read(byte[] b, int off, int len) throws java.io.IOException {
            if (remaining <= 0) return -1;
            int n = super.read(b, off, (int) Math.min(len, remaining));
            if (n > 0) remaining -= n;
            return n;
        }
    }

    /**
     * Launch the system document picker. Results are copied into the app's
     * cache directory so Go receives real filesystem paths. Called by
     * WailsBridge on the main thread.
     */
    public void launchFilePicker(int callbackID, boolean multiple) {
        synchronized (this) {
            if (pendingFilePickerCallbackID != -1) {
                // Only one picker can be in flight
                bridge.filePickerDone(callbackID);
                return;
            }
            pendingFilePickerCallbackID = callbackID;
        }

        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType("*/*");
        intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, multiple);
        try {
            startActivityForResult(intent, FILE_PICKER_REQUEST);
        } catch (Exception e) {
            Log.e(TAG, "Failed to launch file picker", e);
            pendingFilePickerCallbackID = -1;
            bridge.filePickerDone(callbackID);
        }
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, @Nullable Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == PHOTO_CAPTURE_REQUEST || requestCode == VIDEO_CAPTURE_REQUEST) {
            handleCaptureResult(resultCode, data);
            return;
        }
        if (requestCode == SAVE_FILE_REQUEST) {
            handleSaveFileResult(resultCode, data);
            return;
        }
        if (requestCode == SAVE_FOLDER_REQUEST) {
            handleSaveFolderResult(resultCode, data);
            return;
        }
        if (requestCode == PICK_UPLOAD_FOLDER_REQUEST) {
            handlePickUploadFolderResult(resultCode, data);
            return;
        }
        if (requestCode != FILE_PICKER_REQUEST) {
            return;
        }
        final int callbackID = pendingFilePickerCallbackID;
        pendingFilePickerCallbackID = -1;
        if (callbackID == -1) {
            return;
        }

        final List<Uri> uris = new ArrayList<>();
        if (resultCode == RESULT_OK && data != null) {
            if (data.getClipData() != null) {
                for (int i = 0; i < data.getClipData().getItemCount(); i++) {
                    uris.add(data.getClipData().getItemAt(i).getUri());
                }
            } else if (data.getData() != null) {
                uris.add(data.getData());
            }
        }

        // Copy the documents off the main thread, then notify Go
        new Thread(() -> {
            for (Uri uri : uris) {
                String path = copyUriToCache(uri);
                if (path != null) {
                    bridge.filePickerResult(callbackID, path);
                }
            }
            bridge.filePickerDone(callbackID);
        }).start();
    }

    /**
     * Offer a file from the app's private storage through the system save
     * dialog, which lists device storage as well as cloud providers. The share
     * sheet lists only apps, so it has no way to put a file in Downloads.
     */
    public void saveFileAs(final String path) {
        runOnUiThread(() -> {
            pendingSavePaths.add(path);
            if (pendingSavePaths.size() == 1) launchSaveDialog(path);
        });
    }

    private void launchSaveDialog(String path) {
        File file = new File(path);
        Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType(mimeTypeFor(file));
        intent.putExtra(Intent.EXTRA_TITLE, file.getName());
        startActivityForResult(intent, SAVE_FILE_REQUEST);
    }

    private static String mimeTypeFor(File file) {
        String extension = MimeTypeMap.getFileExtensionFromUrl(Uri.fromFile(file).toString());
        String mimeType = MimeTypeMap.getSingleton().getMimeTypeFromExtension(extension);
        return mimeType != null ? mimeType : "application/octet-stream";
    }

    /**
     * Recreate a downloaded folder, with everything in it, inside a folder the
     * user picks. The staged folder sits in a directory of its own, which is
     * removed with it.
     */
    public void saveFolderAs(final String path) {
        runOnUiThread(() -> {
            pendingSaveFolderPaths.add(path);
            if (pendingSaveFolderPaths.size() == 1) launchFolderPicker();
        });
    }

    private void launchFolderPicker() {
        startActivityForResult(new Intent(Intent.ACTION_OPEN_DOCUMENT_TREE), SAVE_FOLDER_REQUEST);
    }

    private void handleSaveFolderResult(int resultCode, @Nullable Intent data) {
        final String path = pendingSaveFolderPaths.poll();
        if (!pendingSaveFolderPaths.isEmpty()) launchFolderPicker();
        if (path == null) return;

        final Uri tree = resultCode == RESULT_OK && data != null ? data.getData() : null;
        new Thread(() -> {
            File source = new File(path);
            if (tree != null) {
                Uri destination = DocumentsContract.buildDocumentUriUsingTree(tree, DocumentsContract.getTreeDocumentId(tree));
                boolean saved = copyFolderToDocument(source, destination);
                String message = saved ? "Saved " + source.getName() : "Could not save all of " + source.getName();
                runOnUiThread(() -> Toast.makeText(this, message, Toast.LENGTH_SHORT).show());
            }
            deleteRecursively(source);
            File stagingDir = source.getParentFile();
            if (stagingDir != null && !stagingDir.delete()) Log.w(TAG, "Could not remove staging folder " + stagingDir);
        }).start();
    }

    private boolean copyFolderToDocument(File folder, Uri parentDocument) {
        try {
            Uri created = DocumentsContract.createDocument(getContentResolver(), parentDocument,
                    DocumentsContract.Document.MIME_TYPE_DIR, folder.getName());
            if (created == null) return false;
            File[] children = folder.listFiles();
            if (children == null) return true;
            boolean allSaved = true;
            for (File child : children) {
                if (child.isDirectory()) {
                    allSaved &= copyFolderToDocument(child, created);
                    continue;
                }
                Uri file = DocumentsContract.createDocument(getContentResolver(), created, mimeTypeFor(child), child.getName());
                allSaved &= file != null && copyFileToUri(child, file);
            }
            return allSaved;
        } catch (Exception e) {
            Log.e(TAG, "Failed to save folder " + folder, e);
            return false;
        }
    }

    /**
     * Let the user pick a folder to upload. The picker grants a document tree,
     * not a path, so the tree is copied into Go's temp directory and the copy's
     * path is reported as "android:folderPicked" (empty when canceled).
     */
    public void pickFolderToUpload() {
        runOnUiThread(() -> startActivityForResult(new Intent(Intent.ACTION_OPEN_DOCUMENT_TREE), PICK_UPLOAD_FOLDER_REQUEST));
    }

    private void handlePickUploadFolderResult(int resultCode, @Nullable Intent data) {
        final Uri tree = resultCode == RESULT_OK && data != null ? data.getData() : null;
        if (tree == null) {
            reportFolderPicked("");
            return;
        }
        new Thread(() -> {
            String rootId = DocumentsContract.getTreeDocumentId(tree);
            String name = queryDisplayName(DocumentsContract.buildDocumentUriUsingTree(tree, rootId));
            File stagingDir = new File(new File(getFilesDir(), "tmp"), "folder-upload-" + System.nanoTime());
            File root = new File(stagingDir, name != null ? name : "folder");
            if (!root.mkdirs() || !copyDocumentTreeToFolder(tree, rootId, root)) {
                deleteRecursively(stagingDir);
                runOnUiThread(() -> Toast.makeText(this, "Could not read the folder", Toast.LENGTH_SHORT).show());
                reportFolderPicked("");
                return;
            }
            reportFolderPicked(root.getAbsolutePath());
        }).start();
    }

    private void reportFolderPicked(String path) {
        try {
            JSONObject o = new JSONObject();
            o.put("path", path);
            if (bridge != null) bridge.emitEvent("android:folderPicked", o.toString());
        } catch (Exception e) {
            Log.e(TAG, "Failed to report picked folder", e);
        }
    }

    private boolean copyDocumentTreeToFolder(Uri tree, String parentId, File destination) {
        Uri children = DocumentsContract.buildChildDocumentsUriUsingTree(tree, parentId);
        String[] columns = {
                DocumentsContract.Document.COLUMN_DOCUMENT_ID,
                DocumentsContract.Document.COLUMN_DISPLAY_NAME,
                DocumentsContract.Document.COLUMN_MIME_TYPE,
        };
        try (Cursor cursor = getContentResolver().query(children, columns, null, null, null)) {
            if (cursor == null) return false;
            boolean allCopied = true;
            while (cursor.moveToNext()) {
                String id = cursor.getString(0);
                File target = new File(destination, new File(cursor.getString(1)).getName());
                if (DocumentsContract.Document.MIME_TYPE_DIR.equals(cursor.getString(2))) {
                    allCopied &= target.mkdirs() && copyDocumentTreeToFolder(tree, id, target);
                } else {
                    allCopied &= copyUriToFile(DocumentsContract.buildDocumentUriUsingTree(tree, id), target);
                }
            }
            return allCopied;
        } catch (Exception e) {
            Log.e(TAG, "Failed to read picked folder", e);
            return false;
        }
    }

    @Nullable
    private String queryDisplayName(Uri document) {
        try (Cursor cursor = getContentResolver().query(document,
                new String[]{DocumentsContract.Document.COLUMN_DISPLAY_NAME}, null, null, null)) {
            if (cursor != null && cursor.moveToFirst()) return new File(cursor.getString(0)).getName();
        } catch (Exception e) {
            Log.e(TAG, "Failed to read folder name", e);
        }
        return null;
    }

    private boolean copyUriToFile(Uri source, File target) {
        try (InputStream in = getContentResolver().openInputStream(source);
             OutputStream out = new FileOutputStream(target)) {
            if (in == null) return false;
            byte[] buf = new byte[64 * 1024];
            int n;
            while ((n = in.read(buf)) > 0) {
                out.write(buf, 0, n);
            }
            return true;
        } catch (Exception e) {
            Log.e(TAG, "Failed to copy " + source, e);
            return false;
        }
    }

    private static void deleteRecursively(File file) {
        File[] children = file.listFiles();
        if (children != null) {
            for (File child : children) deleteRecursively(child);
        }
        if (!file.delete()) Log.w(TAG, "Could not remove staged " + file);
    }

    private void handleSaveFileResult(int resultCode, @Nullable Intent data) {
        final String path = pendingSavePaths.poll();
        String next = pendingSavePaths.peek();
        if (next != null) launchSaveDialog(next);
        if (path == null) return;

        final Uri target = resultCode == RESULT_OK && data != null ? data.getData() : null;
        // The staged copy is unreachable from outside the app, so it is removed
        // whether or not the user picked a destination.
        new Thread(() -> {
            File source = new File(path);
            if (target != null) {
                boolean saved = copyFileToUri(source, target);
                String message = saved ? "Saved " + source.getName() : "Could not save " + source.getName();
                runOnUiThread(() -> Toast.makeText(this, message, Toast.LENGTH_SHORT).show());
            }
            if (!source.delete()) Log.w(TAG, "Could not remove staged file " + path);
        }).start();
    }

    private boolean copyFileToUri(File source, Uri target) {
        try (InputStream in = new FileInputStream(source);
             OutputStream out = getContentResolver().openOutputStream(target)) {
            if (out == null) return false;
            byte[] buf = new byte[64 * 1024];
            int n;
            while ((n = in.read(buf)) > 0) {
                out.write(buf, 0, n);
            }
            return true;
        } catch (Exception e) {
            Log.e(TAG, "Failed to save file to " + target, e);
            return false;
        }
    }

    /**
     * Copy a content URI into the app cache and return its filesystem path.
     */
    @Nullable
    private String copyUriToCache(Uri uri) {
        String name = "document";
        try (Cursor cursor = getContentResolver().query(uri, null, null, null, null)) {
            if (cursor != null && cursor.moveToFirst()) {
                int idx = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME);
                if (idx >= 0 && cursor.getString(idx) != null) {
                    name = new File(cursor.getString(idx)).getName();
                }
            }
        } catch (Exception ignored) {
        }

        try {
            File dir = new File(getCacheDir(), "wails-picker/" + System.nanoTime());
            if (!dir.mkdirs()) {
                return null;
            }
            File out = new File(dir, name);
            try (InputStream in = getContentResolver().openInputStream(uri);
                 OutputStream os = new FileOutputStream(out)) {
                if (in == null) {
                    return null;
                }
                byte[] buf = new byte[64 * 1024];
                int n;
                while ((n = in.read(buf)) > 0) {
                    os.write(buf, 0, n);
                }
            }
            return out.getAbsolutePath();
        } catch (Exception e) {
            Log.e(TAG, "Failed to copy picked document", e);
            return null;
        }
    }

    /**
     * Execute JavaScript in the WebView from the Go side
     */
    public void executeJavaScript(final String js) {
        runOnUiThread(() -> {
            if (webView != null) {
                webView.evaluateJavascript(js, null);
            }
        });
    }

    // ---- System events ---------------------------------------------------
    // Battery/power, screen lock and network connectivity are surfaced to JS as
    // "system:*" events. The OS broadcasts used here (ACTION_BATTERY_CHANGED,
    // SCREEN_OFF, USER_PRESENT, POWER_SAVE_MODE_CHANGED) are protected system
    // broadcasts, so dynamic registration needs no RECEIVER_* export flag.

    private void registerSystemEventReceivers() {
        // Battery + charging state (sticky broadcast: the current value is
        // delivered to the receiver immediately on registration).
        batteryReceiver = new BroadcastReceiver() {
            @Override public void onReceive(Context context, Intent intent) {
                emitBattery(intent);
            }
        };
        registerReceiver(batteryReceiver, new IntentFilter(Intent.ACTION_BATTERY_CHANGED));

        // Low-power (battery saver) mode toggles → re-emit battery with the flag.
        powerSaveReceiver = new BroadcastReceiver() {
            @Override public void onReceive(Context context, Intent intent) {
                emitBattery(registerSticky(Intent.ACTION_BATTERY_CHANGED));
            }
        };
        registerReceiver(powerSaveReceiver,
                new IntentFilter(PowerManager.ACTION_POWER_SAVE_MODE_CHANGED));

        // Screen lock / unlock. SCREEN_OFF ≈ locked; USER_PRESENT = unlocked.
        screenReceiver = new BroadcastReceiver() {
            @Override public void onReceive(Context context, Intent intent) {
                String action = intent.getAction();
                if (Intent.ACTION_SCREEN_OFF.equals(action)) {
                    emitLock(true);
                } else if (Intent.ACTION_USER_PRESENT.equals(action)) {
                    emitLock(false);
                }
            }
        };
        IntentFilter screenFilter = new IntentFilter();
        screenFilter.addAction(Intent.ACTION_SCREEN_OFF);
        screenFilter.addAction(Intent.ACTION_USER_PRESENT);
        registerReceiver(screenReceiver, screenFilter);

        // Network connectivity / transport type / cellular signal strength.
        connectivityManager = (ConnectivityManager) getSystemService(Context.CONNECTIVITY_SERVICE);
        if (connectivityManager != null) {
            networkCallback = new ConnectivityManager.NetworkCallback() {
                @Override public void onAvailable(Network network) { emitNetwork(network); }
                @Override public void onLost(Network network) { emitNetworkDisconnected(); }
                @Override public void onCapabilitiesChanged(Network network, NetworkCapabilities caps) {
                    emitNetwork(network);
                }
            };
            try {
                connectivityManager.registerDefaultNetworkCallback(networkCallback);
            } catch (Exception e) {
                Log.e(TAG, "registerDefaultNetworkCallback failed", e);
            }
        }
    }

    private void unregisterSystemEventReceivers() {
        safeUnregister(batteryReceiver);
        batteryReceiver = null;
        safeUnregister(powerSaveReceiver);
        powerSaveReceiver = null;
        safeUnregister(screenReceiver);
        screenReceiver = null;
        if (connectivityManager != null && networkCallback != null) {
            try {
                connectivityManager.unregisterNetworkCallback(networkCallback);
            } catch (Exception ignored) {
            }
            networkCallback = null;
        }
    }

    private void safeUnregister(BroadcastReceiver r) {
        if (r != null) {
            try {
                unregisterReceiver(r);
            } catch (Exception ignored) {
            }
        }
    }

    /** Read the current sticky value for an action without a standing receiver. */
    @Nullable
    private Intent registerSticky(String action) {
        return registerReceiver(null, new IntentFilter(action));
    }

    /** Push current battery / network / theme so a freshly-loaded UI is populated. */
    private void emitSystemSnapshot() {
        emitBattery(registerSticky(Intent.ACTION_BATTERY_CHANGED));
        if (connectivityManager != null) {
            Network active = connectivityManager.getActiveNetwork();
            if (active != null) {
                emitNetwork(active);
            } else {
                emitNetworkDisconnected();
            }
        }
        emitTheme();
    }

    private void emitBattery(@Nullable Intent batteryStatus) {
        try {
            float level = -1f;
            String state = "unknown";
            if (batteryStatus != null) {
                int lvl = batteryStatus.getIntExtra(BatteryManager.EXTRA_LEVEL, -1);
                int scale = batteryStatus.getIntExtra(BatteryManager.EXTRA_SCALE, -1);
                if (lvl >= 0 && scale > 0) {
                    level = lvl / (float) scale;
                }
                switch (batteryStatus.getIntExtra(BatteryManager.EXTRA_STATUS, -1)) {
                    case BatteryManager.BATTERY_STATUS_CHARGING: state = "charging"; break;
                    case BatteryManager.BATTERY_STATUS_FULL: state = "full"; break;
                    case BatteryManager.BATTERY_STATUS_DISCHARGING:
                    case BatteryManager.BATTERY_STATUS_NOT_CHARGING: state = "unplugged"; break;
                    default: state = "unknown"; break;
                }
            }
            boolean lowPower = false;
            PowerManager pm = (PowerManager) getSystemService(Context.POWER_SERVICE);
            if (pm != null) {
                lowPower = pm.isPowerSaveMode();
            }
            JSONObject o = new JSONObject();
            o.put("level", (double) level);
            o.put("state", state);
            o.put("lowPowerMode", lowPower);
            if (bridge != null) bridge.emitSystemEvent("android:BatteryChanged", o.toString());
        } catch (Exception e) {
            Log.e(TAG, "emitBattery failed", e);
        }
    }

    private void emitNetwork(@Nullable Network network) {
        try {
            boolean connected = false;
            String type = "none";
            boolean metered = false;
            Integer signal = null;
            if (connectivityManager != null && network != null) {
                NetworkCapabilities caps = connectivityManager.getNetworkCapabilities(network);
                if (caps != null) {
                    connected = caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET);
                    if (caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) {
                        type = "wifi";
                    } else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)) {
                        type = "cellular";
                    } else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)) {
                        type = "wired";
                    } else {
                        type = "other";
                    }
                    metered = !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED);
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                        int s = caps.getSignalStrength();
                        if (s != Integer.MIN_VALUE) {
                            signal = s; // dBm; closer to 0 is a stronger signal
                        }
                    }
                }
            }
            JSONObject o = new JSONObject();
            o.put("connected", connected);
            o.put("type", type);
            o.put("metered", metered);
            if (signal != null) {
                o.put("signal", (int) signal);
            }
            if (bridge != null) bridge.emitSystemEvent("android:NetworkChanged", o.toString());
        } catch (Exception e) {
            Log.e(TAG, "emitNetwork failed", e);
        }
    }

    private void emitNetworkDisconnected() {
        try {
            JSONObject o = new JSONObject();
            o.put("connected", false);
            o.put("type", "none");
            o.put("metered", false);
            if (bridge != null) bridge.emitSystemEvent("android:NetworkChanged", o.toString());
        } catch (Exception ignored) {
        }
    }

    private void emitLock(boolean locked) {
        // Lock/unlock are signals (no payload); name carries the state.
        if (bridge != null) {
            bridge.emitSystemEvent(locked ? "android:ScreenLocked" : "android:ScreenUnlocked", "{}");
        }
    }

    private void emitTheme() {
        try {
            int mode = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
            JSONObject o = new JSONObject();
            // "isDarkMode" matches the context key the desktop platforms use.
            o.put("isDarkMode", mode == Configuration.UI_MODE_NIGHT_YES);
            if (bridge != null) bridge.emitSystemEvent("android:ThemeChanged", o.toString());
        } catch (Exception ignored) {
        }
    }

    @Override
    public void onConfigurationChanged(Configuration newConfig) {
        super.onConfigurationChanged(newConfig);
        // Fires for light/dark switches because the manifest lists uiMode in
        // android:configChanges (otherwise the activity would be recreated).
        emitTheme();
        // The WebView takes its text zoom from the font scale only when it is
        // created, and the manifest keeps it across font scale changes.
        if (webView != null) {
            webView.getSettings().setTextZoom(Math.round(newConfig.fontScale * 100));
        }
    }

    @Override
    protected void onStart() {
        super.onStart();
        // Battery: only monitor system events while the app is visible.
        if (!systemReceiversRegistered) {
            registerSystemEventReceivers();
            systemReceiversRegistered = true;
        }
        if (bridge != null) {
            bridge.onStart();
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (bridge != null) {
            bridge.onResume();
        }
    }

    @Override
    protected void onPause() {
        super.onPause();
        if (bridge != null) {
            bridge.onPause();
        }
    }

    @Override
    protected void onStop() {
        super.onStop();
        if (systemReceiversRegistered) {
            unregisterSystemEventReceivers();
            systemReceiversRegistered = false;
        }
        if (bridge != null) {
            bridge.onStop();
        }
    }

    @Override
    public void onLowMemory() {
        super.onLowMemory();
        if (bridge != null) {
            bridge.onLowMemory();
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        unregisterSystemEventReceivers();
        if (bridge != null) {
            if (closingForGood()) {
                bridge.shutdown();
            } else {
                bridge.detach();
            }
        }
        if (newestInstance == this) newestInstance = null;
        if (webView != null) {
            webView.destroy();
        }
    }

    // A job in the foreground service keeps running after the user swipes the
    // task away, so the app shuts down only when no job holds it. A launch that
    // clears the task creates the next activity before this one is destroyed,
    // and the app has to stay up for it.
    private boolean closingForGood() {
        return isFinishing() && !isChangingConfigurations() && !WailsForegroundService.isRunning()
                && newestInstance == this;
    }

    // singleTask hands a link opened while the app runs to this activity rather
    // than a second one with its own WebView and Go runtime.
    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        if (bridge != null && bridge.setPendingDeepLink(intent)) {
            bridge.emitEvent("android:deeplink", "{}");
        }
    }

    // Android 15 enforces edge-to-edge, and adjustResize no longer shrinks the
    // window for the keyboard. The insets passed on shrink by the keyboard's
    // height so the page does not also keep room for the navigation bar the
    // keyboard covers.
    private void resizeForKeyboard(View container) {
        container.setOnApplyWindowInsetsListener((v, insets) -> {
            int keyboardHeight = insets.getInsets(WindowInsets.Type.ime()).bottom;
            v.setPadding(0, 0, 0, keyboardHeight);
            WindowInsets pageInsets = insets.inset(0, 0, 0, keyboardHeight);
            publishSafeArea(pageInsets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout()));
            return pageInsets;
        });
    }

    // Older WebViews, such as 131 on the Galaxy Z Fold4, report 0 for every
    // env(safe-area-inset-*) in an edge-to-edge window, which lays the page out
    // under the system bars. The page takes the larger of env() and these.
    private void publishSafeArea(Insets safeArea) {
        float density = getResources().getDisplayMetrics().density;
        safeAreaScript = String.format(Locale.ROOT,
                "(() => { const s = document.documentElement.style;"
                        + " s.setProperty('--android-safe-area-top', '%.2fpx');"
                        + " s.setProperty('--android-safe-area-right', '%.2fpx');"
                        + " s.setProperty('--android-safe-area-bottom', '%.2fpx');"
                        + " s.setProperty('--android-safe-area-left', '%.2fpx'); })()",
                safeArea.top / density, safeArea.right / density,
                safeArea.bottom / density, safeArea.left / density);
        if (webView != null) webView.evaluateJavascript(safeAreaScript, null);
    }

    // From targetSdk 36 the system no longer calls onBackPressed(), so back
    // navigation has to go through the dispatcher. The frontend drives its screens
    // from React state rather than WebView history, so it decides what back closes.
    private void registerBackNavigation() {
        // Left to the system, back at the root finishes the activity even on
        // Android 12 and later, and the page with it, while Go and any job keep
        // running. Registered first, so the in-app callback below takes priority.
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                moveTaskToBack(true);
            }
        });
        inAppBackNavigation = new OnBackPressedCallback(false) {
            @Override
            public void handleOnBackPressed() {
                if (bridge != null) bridge.emitEvent("android:back", "{}");
            }
        };
        getOnBackPressedDispatcher().addCallback(this, inAppBackNavigation);
    }

    private class AndroidBackBridge {
        @JavascriptInterface
        public void setIntercepting(boolean intercepting) {
            runOnUiThread(() -> {
                if (inAppBackNavigation != null) inAppBackNavigation.setEnabled(intercepting);
            });
        }
    }

    private class TerminalInputBridge {
        @JavascriptInterface
        public void setTerminalFocused(boolean focused) {
            runOnUiThread(() -> {
                if (webView != null) webView.setTerminalFocused(focused);
            });
        }
    }
}

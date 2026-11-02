package com.wails.app;

import android.net.Uri;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;
import java.util.Collections;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import android.webkit.WebView;
import androidx.webkit.JavaScriptReplyProxy;
import androidx.webkit.WebMessageCompat;
import androidx.webkit.WebViewCompat;
import androidx.webkit.WebViewFeature;
import com.wails.app.BuildConfig;
import org.json.JSONException;
import org.json.JSONObject;

/**
 * WailsJSBridge carries messages between the web frontend and the Go backend.
 * The page reaches it as the `window.wails` object, which a document-start
 * script builds on a web message listener.
 *
 * An addJavascriptInterface object is injected into every frame of every page
 * the WebView shows, and its methods cannot tell which frame called them. The
 * listener object is injected only into frames of the app's origin, and this
 * bridge answers only the top-level document, so neither another site nor a
 * frame inside the page can call Go.
 */
public class WailsJSBridge implements WebViewCompat.WebMessageListener {
    private static final String TAG = "WailsJSBridge";
    private static final boolean DEBUG = BuildConfig.DEBUG;
    private static final String NATIVE_OBJECT = "wailsNative";
    // Pooled threads avoid unbounded thread creation under high call volume.
    private static final ExecutorService executor = Executors.newCachedThreadPool();

    // The contract the @wailsio/runtime Android transport expects: invoke and
    // invokeAsync on window.wails, and replies through _wailsAndroidCallback.
    private static final String PAGE_SCRIPT =
            "(() => {\n"
            + "  const nativeBridge = window." + NATIVE_OBJECT + ";\n"
            + "  if (window !== window.top || !nativeBridge) return;\n"
            + "  nativeBridge.onmessage = (event) => {\n"
            + "    const reply = JSON.parse(event.data);\n"
            + "    if (window._wailsAndroidCallback) window._wailsAndroidCallback(reply.id, reply.response, reply.error);\n"
            + "  };\n"
            + "  const send = (message) => nativeBridge.postMessage(JSON.stringify(message));\n"
            + "  window.wails = {\n"
            + "    invoke: (message) => { send({kind: 'message', message}); return '{}'; },\n"
            + "    invokeAsync: (id, payload) => send({kind: 'call', id, payload}),\n"
            + "    log: (level, message) => send({kind: 'log', level, message}),\n"
            + "    platform: () => 'android',\n"
            + "    isDebug: () => " + DEBUG + ",\n"
            + "  };\n"
            + "})();";

    private final WailsBridge bridge;
    private final Handler mainHandler = new Handler(Looper.getMainLooper());

    private WailsJSBridge(WailsBridge bridge) {
        this.bridge = bridge;
    }

    /**
     * Give the pages of appOrigin the bridge. Returns false, and gives no page
     * the bridge, when this WebView cannot limit it to the app's own document.
     */
    public static boolean install(WebView webView, WailsBridge bridge, String appOrigin) {
        if (!WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER)
                || !WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)) {
            return false;
        }
        Set<String> appOriginOnly = Collections.singleton(appOrigin);
        WebViewCompat.addWebMessageListener(webView, NATIVE_OBJECT, appOriginOnly, new WailsJSBridge(bridge));
        WebViewCompat.addDocumentStartJavaScript(webView, PAGE_SCRIPT, appOriginOnly);
        return true;
    }

    @Override
    public void onPostMessage(WebView view, WebMessageCompat message, Uri sourceOrigin,
                              boolean isMainFrame, JavaScriptReplyProxy replyProxy) {
        if (!isMainFrame) {
            Log.w(TAG, "Refused a message from a frame inside the page");
            return;
        }
        String data = message.getData();
        if (data == null) return;
        try {
            JSONObject request = new JSONObject(data);
            switch (request.optString("kind")) {
                case "call":
                    invokeAsync(request.getString("id"), request.getString("payload"), replyProxy);
                    break;
                case "message":
                    final String text = request.getString("message");
                    executor.execute(() -> bridge.handleMessage(text));
                    break;
                case "log":
                    log(request.optString("level"), request.optString("message"));
                    break;
                default:
                    Log.w(TAG, "Unknown message kind: " + request.optString("kind"));
            }
        } catch (JSONException e) {
            Log.e(TAG, "Malformed message from the page", e);
        }
    }

    private void invokeAsync(final String callbackId, final String payload, final JavaScriptReplyProxy replyProxy) {
        if (DEBUG) Log.d(TAG, "InvokeAsync called: " + describeCall(payload));

        // Handle off the UI thread so we don't block the WebView.
        executor.execute(() -> {
            try {
                String response = bridge.handleRuntimeCall(payload);
                sendCallback(replyProxy, callbackId, response, null);
            } catch (Exception e) {
                Log.e(TAG, "Error in async invoke", e);
                sendCallback(replyProxy, callbackId, null, e.getMessage());
            }
        });
    }

    private void log(String level, String message) {
        switch (level.toLowerCase()) {
            case "debug":
                Log.d(TAG + "/JS", message);
                break;
            case "info":
                Log.i(TAG + "/JS", message);
                break;
            case "warn":
                Log.w(TAG + "/JS", message);
                break;
            case "error":
                Log.e(TAG + "/JS", message);
                break;
            default:
                Log.v(TAG + "/JS", message);
                break;
        }
    }

    // The reply goes to the document that made the call, and to no other.
    private void sendCallback(JavaScriptReplyProxy replyProxy, String callbackId, String result, String error) {
        try {
            JSONObject reply = new JSONObject();
            reply.put("id", callbackId);
            reply.put("response", error != null ? JSONObject.NULL : (result != null ? result : ""));
            reply.put("error", error != null ? error : JSONObject.NULL);
            final String message = reply.toString();
            mainHandler.post(() -> replyProxy.postMessage(message));
        } catch (JSONException e) {
            Log.e(TAG, "Could not encode a reply", e);
        }
    }

    // Call arguments carry passwords, so a call is identified by its IDs only.
    private static String describeCall(String payload) {
        try {
            JSONObject call = new JSONObject(payload);
            String description = "object " + call.optInt("object") + " method " + call.optInt("method");
            JSONObject args = call.optJSONObject("args");
            if (args != null && args.has("methodID")) {
                description += " methodID " + args.optLong("methodID");
            }
            return description;
        } catch (JSONException e) {
            return "unparsable payload";
        }
    }
}

package com.wails.app;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.Bundle;
import android.os.IBinder;
import android.os.PowerManager;

import java.util.List;

import androidx.annotation.Nullable;
import androidx.core.app.NotificationCompat;

/**
 * Keeps the process alive while a job the user started on the tablet runs, so
 * the Go side keeps its SSH session when the app is backgrounded. Without it
 * Android freezes the backgrounded process, and the wake lock keeps the CPU
 * running once the screen turns off. The notification shows the job's title,
 * detail and progress, and {@link #update} changes it in place while it runs.
 * From Android 16 it asks to be promoted to a Live Update, which puts it in
 * the status bar and on the lock screen.
 */
public class WailsForegroundService extends android.app.Service {
    public static final String ACTION_START = "com.wails.app.FGS_START";
    private static final String CHANNEL_ID = "wails_foreground";
    private static final int NOTIFICATION_ID = 0x57A1;
    private static final int INDETERMINATE = -1;
    // Notification.EXTRA_REQUEST_PROMOTED_ONGOING, which is public only from API 36.1.
    private static final String EXTRA_REQUEST_PROMOTED_ONGOING = "android.requestPromotedOngoing";

    private static volatile WailsForegroundService running;

    private PowerManager.WakeLock wakeLock;

    public static boolean isRunning() {
        return running != null;
    }

    public static void update(String title, String text, int percent) {
        WailsForegroundService service = running;
        if (service == null) {
            return;
        }
        NotificationManager nm = (NotificationManager) service.getSystemService(NOTIFICATION_SERVICE);
        nm.notify(NOTIFICATION_ID, service.buildNotification(title, text, percent));
    }

    /**
     * Ends the service but leaves its notification, turned into a dismissible
     * one that says the job failed and why, for a user who was not watching.
     */
    public static void finishFailed(String title, String text) {
        WailsForegroundService service = running;
        if (service == null) {
            return;
        }
        service.stopForeground(STOP_FOREGROUND_DETACH);
        NotificationManager nm = (NotificationManager) service.getSystemService(NOTIFICATION_SERVICE);
        nm.notify(NOTIFICATION_ID, new NotificationCompat.Builder(service, CHANNEL_ID)
                .setSmallIcon(R.mipmap.ic_launcher_foreground)
                .setContentTitle(title)
                .setContentText(text)
                .setSubText("Failed")
                .setStyle(new NotificationCompat.BigTextStyle().bigText(text))
                .setContentIntent(service.launchIntent())
                .setAutoCancel(true)
                .build());
        service.stopSelf();
    }

    @Override
    public void onCreate() {
        super.onCreate();
        PowerManager pm = (PowerManager) getSystemService(POWER_SERVICE);
        wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "reManager:backgroundWork");
        wakeLock.setReferenceCounted(false);
        wakeLock.acquire();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        String title = "";
        String text = "";
        int percent = INDETERMINATE;
        if (intent != null) {
            if (intent.getStringExtra("title") != null) title = intent.getStringExtra("title");
            if (intent.getStringExtra("text") != null) text = intent.getStringExtra("text");
            percent = intent.getIntExtra("percent", INDETERMINATE);
        }

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            NotificationManager nm = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
            NotificationChannel ch = new NotificationChannel(
                    CHANNEL_ID, "Jobs in progress", NotificationManager.IMPORTANCE_LOW);
            nm.createNotificationChannel(ch);
        }

        Notification n = buildNotification(title, text, percent);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(NOTIFICATION_ID, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE);
        } else {
            startForeground(NOTIFICATION_ID, n);
        }
        running = this;
        // A restart after the process dies would have no job to show, since
        // the job lived in the process that died.
        return START_NOT_STICKY;
    }

    @Override
    public void onDestroy() {
        running = null;
        if (wakeLock != null && wakeLock.isHeld()) {
            wakeLock.release();
        }
        super.onDestroy();
    }

    private PendingIntent launchIntent() {
        Intent launch = getPackageManager().getLaunchIntentForPackage(getPackageName());
        if (launch == null) {
            return null;
        }
        int piFlags = Build.VERSION.SDK_INT >= Build.VERSION_CODES.M
                ? PendingIntent.FLAG_IMMUTABLE : 0;
        return PendingIntent.getActivity(this, 0, launch, piFlags);
    }

    private Notification buildNotification(String title, String text, int percent) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.BAKLAVA) {
            return buildLiveUpdate(title, text, percent);
        }
        return new NotificationCompat.Builder(this, CHANNEL_ID)
                .setSmallIcon(R.mipmap.ic_launcher_foreground)
                .setContentTitle(title)
                .setContentText(text)
                .setProgress(100, Math.max(percent, 0), percent < 0)
                .setOngoing(true)
                .setOnlyAlertOnce(true)
                .setSilent(true)
                .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
                .setContentIntent(launchIntent())
                .build();
    }

    private Notification buildLiveUpdate(String title, String text, int percent) {
        Notification.ProgressStyle progress = new Notification.ProgressStyle()
                .setProgressSegments(List.of(new Notification.ProgressStyle.Segment(100)))
                .setProgressIndeterminate(percent < 0);
        Notification.Builder builder = new Notification.Builder(this, CHANNEL_ID)
                .setSmallIcon(R.mipmap.ic_launcher_foreground)
                .setContentTitle(title)
                .setContentText(text)
                .setStyle(progress)
                .setOngoing(true)
                .setOnlyAlertOnce(true)
                .setForegroundServiceBehavior(Notification.FOREGROUND_SERVICE_IMMEDIATE)
                .setContentIntent(launchIntent());
        if (percent >= 0) {
            progress.setProgress(percent);
            builder.setShortCriticalText(percent + "%");
        }
        Bundle promotion = new Bundle();
        promotion.putBoolean(EXTRA_REQUEST_PROMOTED_ONGOING, true);
        builder.addExtras(promotion);
        return builder.build();
    }

    @Nullable
    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }
}

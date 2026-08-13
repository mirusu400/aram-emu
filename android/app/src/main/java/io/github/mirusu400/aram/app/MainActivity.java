package io.github.mirusu400.aram.app;

import android.app.Activity;
import android.content.ContentResolver;
import android.content.Intent;
import android.database.Cursor;
import android.media.AudioAttributes;
import android.media.AudioFocusRequest;
import android.media.AudioManager;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.provider.OpenableColumns;
import android.view.View;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.WindowManager;
import android.widget.Toast;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import go.Seq;
import io.github.mirusu400.aram.mobile.EbitenView;
import io.github.mirusu400.aram.mobile.Host;
import io.github.mirusu400.aram.mobile.Mobile;

public final class MainActivity extends Activity
        implements Host, AudioManager.OnAudioFocusChangeListener {
    private static final int REQUEST_DOCUMENT = 1001;
    private static final long MAX_IMPORT_BYTES = 2L * 1024L * 1024L * 1024L;
    private static final String STATE_PENDING_FIRMWARE = "pending_firmware";

    private final ExecutorService importExecutor = Executors.newSingleThreadExecutor();

    private EbitenView gameView;
    private AudioManager audioManager;
    private AudioFocusRequest audioFocusRequest;
    private boolean pendingFirmware;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        Seq.setContext(getApplicationContext());
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        enterImmersiveMode();

        if (savedInstanceState != null) {
            pendingFirmware = savedInstanceState.getBoolean(
                    STATE_PENDING_FIRMWARE,
                    false
            );
        }

        Mobile.configureStorage(getFilesDir().getAbsolutePath());
        Mobile.setHost(this);

        gameView = new EbitenView(this);
        gameView.setFocusable(true);
        gameView.setFocusableInTouchMode(true);
        setContentView(gameView);
        gameView.requestFocus();

        audioManager = (AudioManager) getSystemService(AUDIO_SERVICE);
        handleIncomingIntent(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        handleIncomingIntent(intent);
    }

    @Override
    public void requestDocument(boolean firmware) {
        runOnUiThread(() -> {
            if (isFinishing() || isDestroyed()) {
                Mobile.documentSelectionCanceled();
                return;
            }
            pendingFirmware = firmware;
            Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
            intent.addCategory(Intent.CATEGORY_OPENABLE);
            intent.setType("*/*");
            intent.putExtra(Intent.EXTRA_MIME_TYPES, new String[]{
                    "application/zip",
                    "application/octet-stream"
            });
            intent.addFlags(
                    Intent.FLAG_GRANT_READ_URI_PERMISSION
                            | Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION
            );
            startActivityForResult(intent, REQUEST_DOCUMENT);
        });
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != REQUEST_DOCUMENT) {
            return;
        }
        if (resultCode != RESULT_OK || data == null || data.getData() == null) {
            Mobile.documentSelectionCanceled();
            return;
        }

        Uri uri = data.getData();
        if ((data.getFlags() & Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION) != 0) {
            try {
                getContentResolver().takePersistableUriPermission(
                        uri,
                        Intent.FLAG_GRANT_READ_URI_PERMISSION
                );
            } catch (SecurityException ignored) {
                // Some providers grant access only for the current Activity.
            }
        }
        importDocument(uri, pendingFirmware);
    }

    @Override
    protected void onSaveInstanceState(Bundle outState) {
        outState.putBoolean(STATE_PENDING_FIRMWARE, pendingFirmware);
        super.onSaveInstanceState(outState);
    }

    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        if (hasFocus) {
            enterImmersiveMode();
        }
    }

    // The system bars overlay the frontend's own top menu bar, so the game
    // runs fullscreen; an edge swipe reveals the bars transiently.
    private void enterImmersiveMode() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            WindowManager.LayoutParams attributes = getWindow().getAttributes();
            attributes.layoutInDisplayCutoutMode =
                    WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
            getWindow().setAttributes(attributes);
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            WindowInsetsController controller = getWindow().getInsetsController();
            if (controller != null) {
                controller.setSystemBarsBehavior(
                        WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
                );
                controller.hide(WindowInsets.Type.systemBars());
            }
        } else {
            enterLegacyImmersiveMode();
        }
    }

    @SuppressWarnings("deprecation")
    private void enterLegacyImmersiveMode() {
        getWindow().getDecorView().setSystemUiVisibility(
                View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
                        | View.SYSTEM_UI_FLAG_FULLSCREEN
                        | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                        | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                        | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
                        | View.SYSTEM_UI_FLAG_LAYOUT_STABLE
        );
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (gameView != null) {
            gameView.resumeGame();
            gameView.requestFocus();
        }
        Mobile.resume();
        requestAudioFocus();
    }

    @Override
    protected void onPause() {
        Mobile.pause();
        Mobile.audioFocus(false);
        abandonAudioFocus();
        if (gameView != null) {
            gameView.suspendGame();
        }
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        Mobile.setHost(null);
        importExecutor.shutdownNow();
        super.onDestroy();
    }

    @Override
    public void onAudioFocusChange(int focusChange) {
        boolean active = focusChange == AudioManager.AUDIOFOCUS_GAIN;
        Mobile.audioFocus(active);
    }

    private void requestAudioFocus() {
        if (audioManager == null) {
            Mobile.audioFocus(false);
            return;
        }
        int result;
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            if (audioFocusRequest == null) {
                AudioAttributes attributes = new AudioAttributes.Builder()
                        .setUsage(AudioAttributes.USAGE_GAME)
                        .setContentType(AudioAttributes.CONTENT_TYPE_MUSIC)
                        .build();
                audioFocusRequest = new AudioFocusRequest.Builder(
                        AudioManager.AUDIOFOCUS_GAIN
                )
                        .setAudioAttributes(attributes)
                        .setOnAudioFocusChangeListener(this)
                        .build();
            }
            result = audioManager.requestAudioFocus(audioFocusRequest);
        } else {
            result = requestLegacyAudioFocus();
        }
        Mobile.audioFocus(result == AudioManager.AUDIOFOCUS_REQUEST_GRANTED);
    }

    private void abandonAudioFocus() {
        if (audioManager == null) {
            return;
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            if (audioFocusRequest != null) {
                audioManager.abandonAudioFocusRequest(audioFocusRequest);
            }
        } else {
            abandonLegacyAudioFocus();
        }
    }

    @SuppressWarnings("deprecation")
    private int requestLegacyAudioFocus() {
        return audioManager.requestAudioFocus(
                this,
                AudioManager.STREAM_MUSIC,
                AudioManager.AUDIOFOCUS_GAIN
        );
    }

    @SuppressWarnings("deprecation")
    private void abandonLegacyAudioFocus() {
        audioManager.abandonAudioFocus(this);
    }

    private void handleIncomingIntent(Intent intent) {
        if (intent == null) {
            return;
        }
        Uri uri = null;
        if (Intent.ACTION_VIEW.equals(intent.getAction())) {
            uri = intent.getData();
        } else if (Intent.ACTION_SEND.equals(intent.getAction())) {
            Object stream = intent.getParcelableExtra(Intent.EXTRA_STREAM);
            if (stream instanceof Uri) {
                uri = (Uri) stream;
            }
        }
        if (uri != null) {
            importDocument(uri, false);
        }
    }

    private void importDocument(Uri uri, boolean firmware) {
        importExecutor.execute(() -> {
            try {
                ImportedDocument document = copyIntoPrivateStorage(uri);
                runOnUiThread(() -> {
                    if (firmware) {
                        Mobile.openFirmware(
                                document.file.getAbsolutePath(),
                                document.displayName
                        );
                    } else {
                        Mobile.openDocument(
                                document.file.getAbsolutePath(),
                                document.displayName
                        );
                    }
                });
            } catch (Exception error) {
                runOnUiThread(() -> {
                    Mobile.documentSelectionCanceled();
                    String detail = error.getMessage();
                    if (detail == null || detail.isEmpty()) {
                        detail = error.getClass().getSimpleName();
                    }
                    Toast.makeText(
                            this,
                            getString(R.string.import_failed, detail),
                            Toast.LENGTH_LONG
                    ).show();
                });
            }
        });
    }

    private ImportedDocument copyIntoPrivateStorage(Uri uri) throws IOException {
        ContentResolver resolver = getContentResolver();
        String displayName = queryDisplayName(resolver, uri);
        File imports = new File(getFilesDir(), "imports");
        if (!imports.isDirectory() && !imports.mkdirs()) {
            throw new IOException("cannot create the import directory");
        }

        String safeName = safeFileName(displayName);
        File destination = new File(
                imports,
                UUID.randomUUID() + "-" + safeName
        );
        File temporary = new File(destination.getAbsolutePath() + ".part");

        long total = 0;
        try (
                InputStream raw = resolver.openInputStream(uri);
                BufferedInputStream input = raw == null
                        ? null
                        : new BufferedInputStream(raw);
                FileOutputStream fileOutput = new FileOutputStream(temporary);
                BufferedOutputStream output = new BufferedOutputStream(fileOutput)
        ) {
            if (input == null) {
                throw new IOException("the document provider returned no data");
            }
            byte[] buffer = new byte[64 * 1024];
            for (int count; (count = input.read(buffer)) != -1; ) {
                total += count;
                if (total > MAX_IMPORT_BYTES) {
                    throw new IOException("the selected document exceeds 2 GiB");
                }
                output.write(buffer, 0, count);
            }
            output.flush();
            fileOutput.getFD().sync();
        } catch (IOException | RuntimeException error) {
            temporary.delete();
            throw error;
        }

        if (!temporary.renameTo(destination)) {
            temporary.delete();
            throw new IOException("cannot finish the imported document");
        }
        return new ImportedDocument(destination, displayName);
    }

    private static String queryDisplayName(ContentResolver resolver, Uri uri) {
        try (
                Cursor cursor = resolver.query(
                        uri,
                        new String[]{OpenableColumns.DISPLAY_NAME},
                        null,
                        null,
                        null
                )
        ) {
            if (cursor != null && cursor.moveToFirst()) {
                int index = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME);
                if (index >= 0) {
                    String name = cursor.getString(index);
                    if (name != null && !name.isEmpty()) {
                        return name;
                    }
                }
            }
        } catch (RuntimeException ignored) {
            // Fall back to the URI when a provider does not implement query().
        }
        String fallback = uri.getLastPathSegment();
        return fallback == null || fallback.isEmpty() ? "document" : fallback;
    }

    private static String safeFileName(String name) {
        String result = name.replaceAll(
                "[\\\\/:*?\"<>|\\p{Cntrl}]",
                "_"
        ).trim();
        if (result.isEmpty() || ".".equals(result) || "..".equals(result)) {
            result = "document";
        }
        if (result.length() > 120) {
            result = result.substring(result.length() - 120);
        }
        return result;
    }

    private static final class ImportedDocument {
        final File file;
        final String displayName;

        ImportedDocument(File file, String displayName) {
            this.file = file;
            this.displayName = displayName;
        }
    }
}

package io.github.mirusu400.aram.app;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;

import java.io.File;
import java.io.FileNotFoundException;

/**
 * Hands a verified product package to the system package installer. The Go
 * frontend keeps the package on disk and stays running; Android replaces the
 * app only after the user confirms in the installer, which reads the package
 * through {@link UpdateProvider}.
 *
 * <p>This is the sideload (GitHub) flavor. The Google Play flavor swaps in a
 * stub that refuses in-app installation, because Google Play delivers updates
 * and REQUEST_INSTALL_PACKAGES is against Play policy.
 */
final class UpdateInstaller {
    private UpdateInstaller() {
    }

    static void install(Activity activity, String path) throws Exception {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.N) {
            // Earlier package installers accept only file: URIs, and the
            // app-private update folder is not readable by other apps.
            throw new UnsupportedOperationException(
                    "in-app package installation needs Android 7.0 or newer"
            );
        }
        if (path == null || path.isEmpty()) {
            throw new FileNotFoundException("no package path was provided");
        }
        File file = new File(path).getCanonicalFile();
        if (!file.isFile()) {
            throw new FileNotFoundException(file.getPath());
        }
        Uri uri = UpdateProvider.uriFor(activity, file);
        Intent intent = new Intent(Intent.ACTION_VIEW);
        intent.setDataAndType(uri, UpdateProvider.PACKAGE_MIME_TYPE);
        intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        activity.startActivity(intent);
    }
}

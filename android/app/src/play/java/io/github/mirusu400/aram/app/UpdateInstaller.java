package io.github.mirusu400.aram.app;

import android.app.Activity;

/**
 * Google Play flavor: no in-app updater. Google Play delivers updates, and a
 * self-install path (REQUEST_INSTALL_PACKAGES plus a package-install intent) is
 * against Play policy, so neither the permission, the {@code UpdateProvider},
 * nor a working installer ships in this flavor.
 *
 * <p>The frontend's self-update UI is compiled out of the Play build (the bound
 * aar is built with {@code SelfUpdateDisabled}), so nothing calls this method.
 * It stays as a defensive backstop and always refuses.
 */
final class UpdateInstaller {
    private UpdateInstaller() {
    }

    static void install(Activity activity, String path) throws Exception {
        throw new UnsupportedOperationException(
                "in-app updates are disabled in the Google Play build"
        );
    }
}

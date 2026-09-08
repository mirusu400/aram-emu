package io.github.mirusu400.aram.app;

import android.widget.LinearLayout;

/**
 * Sideloaded builds never request advertising. Keeping this no-op controller
 * in the GitHub flavor also keeps the Google Mobile Ads and UMP SDKs out of
 * those APKs entirely.
 */
final class AdMobController {
    AdMobController(MainActivity activity, LinearLayout adContainer) {
    }

    void start() {
    }

    void onResume() {
    }

    void onPause() {
    }

    void destroy() {
    }
}

package io.github.mirusu400.aram.app;

import android.app.Activity;
import android.util.Log;
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.LinearLayout;

import com.google.android.gms.ads.AdListener;
import com.google.android.gms.ads.AdRequest;
import com.google.android.gms.ads.AdSize;
import com.google.android.gms.ads.AdView;
import com.google.android.gms.ads.LoadAdError;
import com.google.android.gms.ads.MobileAds;
import com.google.android.ump.ConsentInformation;
import com.google.android.ump.ConsentRequestParameters;
import com.google.android.ump.UserMessagingPlatform;

/**
 * Owns Play-only banner monetization. Ads are requested only after UMP has
 * refreshed consent information and says an ad request is permitted.
 */
final class AdMobController {
    private static final String TAG = "aram-admob";

    private final Activity activity;
    private final LinearLayout adContainer;
    private final FrameLayout bannerContainer;
    private final ConsentInformation consentInformation;

    private AdView adView;
    private Button privacyOptionsButton;
    private boolean adsInitialized;

    AdMobController(Activity activity, LinearLayout adContainer) {
        this.activity = activity;
        this.adContainer = adContainer;
        bannerContainer = new FrameLayout(activity);
        adContainer.addView(bannerContainer, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT
        ));
        consentInformation = UserMessagingPlatform.getConsentInformation(activity);
    }

    void start() {
        ConsentRequestParameters parameters = new ConsentRequestParameters.Builder().build();
        // Consent status can expire, so refresh it on every app launch rather
        // than relying on a cached decision.
        consentInformation.requestConsentInfoUpdate(
                activity,
                parameters,
                () -> UserMessagingPlatform.loadAndShowConsentFormIfRequired(
                        activity,
                        formError -> {
                            if (formError != null) {
                                Log.w(TAG, "Consent form unavailable: " + formError.getMessage());
                            }
                            updatePrivacyOptionsEntryPoint();
                            requestAdsIfPermitted();
                        }
                ),
                requestConsentError -> {
                    Log.w(TAG, "Consent information update failed: " + requestConsentError.getMessage());
                    updatePrivacyOptionsEntryPoint();
                    // A valid stored consent decision may still permit ads.
                    requestAdsIfPermitted();
                }
        );
        // A previous valid consent decision can permit requests before the
        // asynchronous update finishes. The guard prevents duplicate loads.
        requestAdsIfPermitted();
    }

    void onResume() {
        if (adView != null) {
            adView.resume();
        }
    }

    void onPause() {
        if (adView != null) {
            adView.pause();
        }
    }

    void destroy() {
        if (adView != null) {
            adView.destroy();
            adView = null;
        }
    }

    private void requestAdsIfPermitted() {
        if (!consentInformation.canRequestAds() || adsInitialized) {
            return;
        }
        adsInitialized = true;
        MobileAds.initialize(
                activity,
                initializationStatus -> activity.runOnUiThread(this::loadBanner)
        );
    }

    private void loadBanner() {
        if (activity.isFinishing() || activity.isDestroyed() || adView != null) {
            return;
        }
        AdView banner = new AdView(activity);
        banner.setAdUnitId(BuildConfig.ADMOB_BANNER_AD_UNIT_ID);
        banner.setAdSize(AdSize.getCurrentOrientationAnchoredAdaptiveBannerAdSize(
                activity,
                currentAdWidth()
        ));
        banner.setAdListener(new AdListener() {
            @Override
            public void onAdFailedToLoad(LoadAdError adError) {
                Log.w(TAG, "Banner failed to load: " + adError.getMessage());
            }
        });

        adView = banner;
        FrameLayout.LayoutParams layout = new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.WRAP_CONTENT,
                FrameLayout.LayoutParams.WRAP_CONTENT,
                Gravity.CENTER_HORIZONTAL
        );
        bannerContainer.addView(banner, layout);
        banner.loadAd(new AdRequest.Builder().build());
    }

    private int currentAdWidth() {
        int containerWidth = bannerContainer.getWidth();
        if (containerWidth <= 0) {
            containerWidth = activity.getResources().getDisplayMetrics().widthPixels;
        }
        float density = activity.getResources().getDisplayMetrics().density;
        return Math.max(1, Math.round(containerWidth / density));
    }

    private void updatePrivacyOptionsEntryPoint() {
        boolean required = consentInformation.getPrivacyOptionsRequirementStatus()
                == ConsentInformation.PrivacyOptionsRequirementStatus.REQUIRED;
        if (!required) {
            if (privacyOptionsButton != null) {
                privacyOptionsButton.setVisibility(View.GONE);
            }
            return;
        }
        if (privacyOptionsButton == null) {
            privacyOptionsButton = new Button(activity);
            privacyOptionsButton.setText(R.string.privacy_options);
            privacyOptionsButton.setContentDescription(
                    activity.getString(R.string.privacy_options)
            );
            privacyOptionsButton.setOnClickListener(view ->
                    UserMessagingPlatform.showPrivacyOptionsForm(
                            activity,
                            formError -> {
                                if (formError != null) {
                                    Log.w(
                                            TAG,
                                            "Privacy options unavailable: " + formError.getMessage()
                                    );
                                }
                                updatePrivacyOptionsEntryPoint();
                            }
                    )
            );
            LinearLayout.LayoutParams layout = new LinearLayout.LayoutParams(
                    LinearLayout.LayoutParams.WRAP_CONTENT,
                    LinearLayout.LayoutParams.WRAP_CONTENT
            );
            layout.gravity = Gravity.END;
            adContainer.addView(privacyOptionsButton, 0, layout);
        }
        privacyOptionsButton.setVisibility(View.VISIBLE);
    }
}

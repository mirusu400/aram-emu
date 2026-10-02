# App-specific R8 rules.
#
# The ebitenmobile-generated AAR contributes consumer rules that retain the Go
# binding packages used through JNI. Android Gradle Plugin and the Google Ads
# dependencies contribute their own rules for manifest components and SDK
# entry points.

# WorkManager starts before MainActivity through AndroidX Startup. Room creates
# WorkDatabase_Impl by reflection, but R8 can remove its unused constructor
# while leaving the class in the DEX. That crashes every Play cold start.
-keep class androidx.work.impl.WorkDatabase_Impl { public <init>(); }

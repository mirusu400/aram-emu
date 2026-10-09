package io.github.mirusu400.aram.app;

import android.annotation.TargetApi;
import android.content.Context;
import android.os.Build;
import android.os.PerformanceHintManager;
import android.os.Process;
import android.util.Log;

// Call frame methods from Go's locked worker, not the Activity UI thread.
// Lifecycle methods retire sessions when the Activity is paused or destroyed.
final class PerformanceTuner {
    interface HintSession {
        void updateTarget(long nanos);
        void report(long nanos);
        void close();
    }

    interface Platform {
        int threadId();
        void setPriority(int priority);
        HintSession openSession(int tid, long targetNanos);
        void warn(String message, RuntimeException error);
    }

    private final Platform platform;
    private HintSession session;
    private int workerTid;
    private int requestedPriority = Integer.MIN_VALUE;
    private long targetNanos;
    private boolean active;
    private boolean closed;
    private boolean hintsUnavailable;

    PerformanceTuner(Context context) {
        this(new AndroidPlatform(context.getApplicationContext()));
    }

    PerformanceTuner(Platform platform) {
        this.platform = platform;
    }

    synchronized void setActive(boolean enabled) {
        active = enabled && !closed;
        if (!active) {
            closeSession();
        }
    }

    synchronized void beginFrame(long target, boolean uiPriority) {
        if (!active || closed || target <= 0) {
            return;
        }
        int tid = platform.threadId();
        if (tid != workerTid) {
            closeSession();
            workerTid = tid;
            requestedPriority = Integer.MIN_VALUE;
        }
        int priority = uiPriority ? Process.THREAD_PRIORITY_DEFAULT : Process.THREAD_PRIORITY_DISPLAY;
        if (priority != requestedPriority) {
            requestedPriority = priority;
            try {
                platform.setPriority(priority);
            } catch (RuntimeException error) {
                platform.warn("emulation thread priority unavailable", error);
            }
        }
        if (hintsUnavailable) {
            return;
        }
        try {
            if (session == null) {
                session = platform.openSession(tid, target);
                hintsUnavailable = session == null;
            } else if (target != targetNanos) {
                session.updateTarget(target);
            }
            targetNanos = target;
        } catch (RuntimeException error) {
            disableHints(error);
        }
    }

    synchronized void endFrame(long actual) {
        if (!active || session == null || actual <= 0) {
            return;
        }
        try {
            session.report(actual);
        } catch (RuntimeException error) {
            disableHints(error);
        }
    }

    void prepareAudioThread() {
        try {
            // Only the lightweight PCM supply pump, never the UI or guest.
            platform.setPriority(Process.THREAD_PRIORITY_AUDIO);
        } catch (RuntimeException error) {
            platform.warn("audio pump priority unavailable", error);
        }
    }

    synchronized void close() {
        closed = true;
        active = false;
        closeSession();
    }

    private void disableHints(RuntimeException error) {
        hintsUnavailable = true;
        closeSession();
        platform.warn("performance hints unavailable", error);
    }

    private void closeSession() {
        HintSession previous = session;
        session = null;
        targetNanos = 0;
        if (previous != null) {
            try {
                previous.close();
            } catch (RuntimeException error) {
                platform.warn("performance hint close failed", error);
            }
        }
    }

    private static final class AndroidPlatform implements Platform {
        private final Context context;

        AndroidPlatform(Context context) {
            this.context = context;
        }

        @Override
        public int threadId() {
            return Process.myTid();
        }

        @Override
        public void setPriority(int priority) {
            Process.setThreadPriority(priority);
        }

        @Override
        public HintSession openSession(int tid, long target) {
            return Build.VERSION.SDK_INT >= 31 ? Api31.open(context, tid, target) : null;
        }

        @Override
        public void warn(String message, RuntimeException error) {
            Log.w("ARAM-Performance", message, error);
        }
    }

    @TargetApi(31)
    private static final class Api31 {
        static HintSession open(Context context, int tid, long target) {
            PerformanceHintManager manager = context.getSystemService(PerformanceHintManager.class);
            if (manager == null) {
                return null;
            }
            PerformanceHintManager.Session nativeSession = manager.createHintSession(new int[]{tid}, target);
            if (nativeSession == null) {
                return null;
            }
            Log.i("ARAM-Performance", "frame hint session opened tid=" + tid + " target_ns=" + target);
            return new HintSession() {
                @Override
                public void updateTarget(long nanos) {
                    nativeSession.updateTargetWorkDuration(nanos);
                }

                @Override
                public void report(long nanos) {
                    nativeSession.reportActualWorkDuration(nanos);
                }

                @Override
                public void close() {
                    nativeSession.close();
                }
            };
        }
    }
}

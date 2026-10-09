package io.github.mirusu400.aram.app;

import org.junit.Test;
import static org.junit.Assert.*;

public class PerformanceTunerTest {
    private static final class Session implements PerformanceTuner.HintSession {
        long target;
        long actual;
        int closes;
        boolean failReport;
        @Override public void updateTarget(long nanos) { target = nanos; }
        @Override public void report(long nanos) {
            if (failReport) throw new IllegalStateException("unavailable");
            actual = nanos;
        }
        @Override public void close() { closes++; }
    }

    private static final class Platform implements PerformanceTuner.Platform {
        int tid = 42;
        int priority;
        int opens;
        boolean supported = true;
        Session session;
        @Override public int threadId() { return tid; }
        @Override public void setPriority(int value) { priority = value; }
        @Override public PerformanceTuner.HintSession openSession(int thread, long target) {
            assertEquals(tid, thread);
            opens++;
            if (!supported) return null;
            session = new Session();
            session.target = target;
            return session;
        }
        @Override public void warn(String message, RuntimeException error) { }
    }

    @Test public void sessionUsesWorkerAndUpdatesTarget() {
        Platform platform = new Platform();
        PerformanceTuner tuner = new PerformanceTuner(platform);
        tuner.setActive(true);
        tuner.beginFrame(16_000_000, false);
        tuner.endFrame(12_000_000);
        assertEquals(-4, platform.priority);
        assertEquals(12_000_000, platform.session.actual);
        tuner.beginFrame(8_000_000, true);
        assertEquals(0, platform.priority);
        assertEquals(8_000_000, platform.session.target);
        assertEquals(1, platform.opens);
    }

    @Test public void pauseRetiresSessionAndResumeReopens() {
        Platform platform = new Platform();
        PerformanceTuner tuner = new PerformanceTuner(platform);
        tuner.setActive(true);
        tuner.beginFrame(16_000_000, false);
        Session first = platform.session;
        tuner.setActive(false);
        tuner.endFrame(12_000_000);
        tuner.beginFrame(16_000_000, false);
        assertEquals(1, first.closes);
        assertEquals(0, first.actual);
        assertEquals(1, platform.opens);
        tuner.setActive(true);
        tuner.beginFrame(16_000_000, false);
        assertEquals(2, platform.opens);
        tuner.close();
        tuner.setActive(true);
        tuner.beginFrame(16_000_000, false);
        assertEquals(2, platform.opens);
        assertEquals(1, platform.session.closes);
    }

    @Test public void unavailableHintsDoNotRetryEachFrame() {
        Platform platform = new Platform();
        platform.supported = false;
        PerformanceTuner tuner = new PerformanceTuner(platform);
        tuner.setActive(true);
        for (int i = 0; i < 60; i++) tuner.beginFrame(16_000_000, false);
        assertEquals(1, platform.opens);
        assertEquals(-4, platform.priority);
    }

    @Test public void failedReportFallsBackAndClosesSession() {
        Platform platform = new Platform();
        PerformanceTuner tuner = new PerformanceTuner(platform);
        tuner.setActive(true);
        tuner.beginFrame(16_000_000, false);
        platform.session.failReport = true;
        tuner.endFrame(12_000_000);
        tuner.beginFrame(16_000_000, false);
        assertEquals(1, platform.session.closes);
        assertEquals(1, platform.opens);
    }

    @Test public void audioPumpUsesAudioPriority() {
        Platform platform = new Platform();
        new PerformanceTuner(platform).prepareAudioThread();
        assertEquals(-16, platform.priority);
    }
}

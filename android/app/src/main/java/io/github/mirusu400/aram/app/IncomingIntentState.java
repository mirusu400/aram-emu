package io.github.mirusu400.aram.app;

import java.util.UUID;

/** Separates an Activity recreation from a new launch or a new Go runtime. */
final class IncomingIntentState {
    private final boolean sameProcess;
    private String requestId;

    IncomingIntentState(String process, String savedProcess, String savedRequestId) {
        sameProcess = process.equals(savedProcess);
        requestId = savedRequestId == null ? newId() : savedRequestId;
    }

    boolean shouldHandleInitialIntent() {
        return !sameProcess;
    }

    String requestId() {
        return requestId;
    }

    void newIntent() {
        requestId = newId();
    }

    private static String newId() {
        return UUID.randomUUID().toString();
    }
}

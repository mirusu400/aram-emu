package io.github.mirusu400.aram.app;

/** Recovery policy for pending imports after the Go runtime has restarted. */
final class PendingImportRecovery {
    private boolean restoresSession;
    private boolean canceledBackup;

    void restore(String kind, Runnable resume) {
        if ("save-backup".equals(kind)) {
            // Its destination is the previous runtime's open game, which is gone.
            canceledBackup = true;
            return;
        }
        resume.run();
        restoresSession |= "input".equals(kind) || "firmware".equals(kind);
    }

    boolean restoresSession() {
        return restoresSession;
    }

    boolean canceledBackup() {
        return canceledBackup;
    }
}

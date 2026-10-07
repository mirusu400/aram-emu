package io.github.mirusu400.aram.app;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

import org.junit.Test;

import static org.junit.Assert.*;

public class PendingImportRecoveryTest {
    @Test
    public void pendingFirmwareOpensANewSessionAndSuppressesTheOldLaunch() {
        PendingImportRecovery recovery = new PendingImportRecovery();
        List<String> resumed = new ArrayList<>();
        recovery.restore("firmware", () -> resumed.add("new-firmware"));
        assertEquals(Arrays.asList("new-firmware"), resumed);
        assertTrue(recovery.restoresSession());
        assertFalse(recovery.canceledBackup());
    }

    @Test
    public void pendingInputOpensANewSessionAndSuppressesTheOldLaunch() {
        PendingImportRecovery recovery = new PendingImportRecovery();
        List<String> resumed = new ArrayList<>();
        recovery.restore("input", () -> resumed.add("new-game"));
        assertEquals(Arrays.asList("new-game"), resumed);
        assertTrue(recovery.restoresSession());
    }

    @Test
    public void pendingBackupIsCanceledAfterProcessDeathRatherThanAppliedToAnEmptyRuntime() {
        PendingImportRecovery recovery = new PendingImportRecovery();
        recovery.restore("save-backup", () -> fail("backup cannot restore its target session"));
        assertTrue(recovery.canceledBackup());
        assertFalse(recovery.restoresSession());
    }

    @Test
    public void mixedRequestsPreserveSessionOrderAndStillCancelTheOldBackup() {
        PendingImportRecovery recovery = new PendingImportRecovery();
        List<String> resumed = new ArrayList<>();
        recovery.restore("input", () -> resumed.add("new-game"));
        recovery.restore("save-backup", () -> fail("backup must be reselected for the target game"));
        recovery.restore("firmware", () -> resumed.add("new-firmware"));
        assertEquals(Arrays.asList("new-game", "new-firmware"), resumed);
        assertTrue(recovery.restoresSession());
        assertTrue(recovery.canceledBackup());
    }
}

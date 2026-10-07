package io.github.mirusu400.aram.app;

import org.junit.Test;

import static org.junit.Assert.*;

public class IncomingIntentStateTest {
    @Test
    public void recreationDoesNotReplayAnAlreadyHandledLaunch() {
        IncomingIntentState first = new IncomingIntentState("process", null, null);
        assertTrue(first.shouldHandleInitialIntent());
        IncomingIntentState recreated = new IncomingIntentState(
                "process", "process", first.requestId()
        );
        assertFalse(recreated.shouldHandleInitialIntent());
        assertEquals(first.requestId(), recreated.requestId());
    }

    @Test
    public void processDeathReopensTheSameImportInsteadOfCreatingAnotherCopy() {
        IncomingIntentState first = new IncomingIntentState("old-process", null, null);
        IncomingIntentState restored = new IncomingIntentState(
                "new-process", "old-process", first.requestId()
        );
        assertTrue(restored.shouldHandleInitialIntent());
        assertEquals(first.requestId(), restored.requestId());
    }

    @Test
    public void explicitNewIntentCreatesANewRequestEvenForTheSameDocument() {
        IncomingIntentState state = new IncomingIntentState("process", null, null);
        String initial = state.requestId();
        state.newIntent();
        assertNotEquals(initial, state.requestId());
    }
}

package io.github.mirusu400.aram.app;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

import org.junit.Test;

import static org.junit.Assert.*;

public class HeldControlsTest {
    @Test
    public void pauseReleasesTouchKeysAndHatAndRepeatedCleanupIsSafe() {
        List<String> events = new ArrayList<>();
        HeldControls controls = recording(events);
        controls.update("touch", -1, "num1");
        controls.update("key", 4, "ok");
        controls.update("hat", 4, "left");
        controls.releaseAll();
        assertEquals(6, events.size());
        assertTrue(events.containsAll(Arrays.asList("num1:false", "ok:false", "left:false")));
        controls.releaseAll();
        controls.update("touch", -1, null);
        assertEquals(6, events.size());
        controls.update("hat", 4, "left");
        assertEquals("left:true", events.get(6));
    }

    @Test
    public void removingADeviceKeepsOverlappingTouchAndOtherDeviceHeld() {
        List<String> events = new ArrayList<>();
        HeldControls controls = recording(events);
        controls.update("key-one", 1, "ok");
        controls.update("key-two", 2, "ok");
        controls.update("touch", -1, "ok");
        controls.update("hat-one", 1, "left");
        controls.releaseDevice(1);
        assertEquals(Arrays.asList("ok:true", "left:true", "left:false"), events);
        controls.releaseDevice(2);
        assertEquals(3, events.size());
        controls.update("touch", -1, null);
        assertEquals("ok:false", events.get(3));
    }

    @Test
    public void hatChangeAndKeyRepeatDoNotProduceDuplicatePresses() {
        List<String> events = new ArrayList<>();
        HeldControls controls = recording(events);
        controls.update("hat-x", 4, "left");
        controls.update("hat-x", 4, "left");
        controls.update("key-left", 4, "left");
        controls.update("hat-x", 4, "right");
        controls.update("key-left", 4, null);
        controls.update("hat-x", 4, null);
        assertEquals(Arrays.asList("left:true", "right:true", "left:false", "right:false"), events);
    }

    private static HeldControls recording(List<String> events) {
        return new HeldControls((control, pressed) -> events.add(control + ":" + pressed));
    }
}

package io.github.mirusu400.aram.app;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.Map;
import java.util.Set;

/** Tracks each touch, key and hat separately, including overlapping controls. */
final class HeldControls {
    interface Sink {
        void press(String control, boolean pressed);
    }

    private static final class Held {
        final int device;
        final String control;

        Held(int device, String control) {
            this.device = device;
            this.control = control;
        }
    }

    private final Sink sink;
    private final Map<Object, Held> held = new HashMap<>();

    HeldControls(Sink sink) {
        this.sink = sink;
    }

    void update(Object source, int device, String control) {
        Held previous = held.get(source);
        if (previous != null && previous.control.equals(control)) {
            return;
        }
        held.remove(source);
        if (previous != null && !isHeld(previous.control)) {
            sink.press(previous.control, false);
        }
        if (control != null) {
            boolean alreadyHeld = isHeld(control);
            held.put(source, new Held(device, control));
            if (!alreadyHeld) {
                sink.press(control, true);
            }
        }
    }

    void releaseDevice(int device) {
        for (Object source : new ArrayList<>(held.keySet())) {
            if (held.get(source).device == device) {
                update(source, device, null);
            }
        }
    }

    void releaseAll() {
        Set<String> controls = new HashSet<>();
        for (Held input : held.values()) {
            controls.add(input.control);
        }
        held.clear();
        for (String control : controls) {
            sink.press(control, false);
        }
    }

    private boolean isHeld(String control) {
        for (Held input : held.values()) {
            if (input.control.equals(control)) {
                return true;
            }
        }
        return false;
    }
}

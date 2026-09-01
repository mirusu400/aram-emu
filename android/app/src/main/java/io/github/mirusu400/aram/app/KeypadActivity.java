package io.github.mirusu400.aram.app;

import android.app.Activity;
import android.graphics.Color;
import android.os.Bundle;
import android.view.Gravity;
import android.view.InputDevice;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.Space;

import java.util.Objects;

import io.github.mirusu400.aram.mobile.Mobile;

/**
 * The virtual keypad on the handset's second physical panel.
 *
 * RG DS and similar RK3568 clamshells expose their two panels as two Android
 * displays. The game runs in {@link MainActivity} on one panel; this Activity
 * owns the keypad on the other. Both live in the same process, so a key feeds
 * the shared Go frontend directly through {@link Mobile#pressControl} - there
 * is no second Ebitengine surface, and the game loop keeps running in its own
 * Activity. Control names match the frontend's on-screen deck, so a press here
 * is indistinguishable from a press on the built-in touch layout.
 */
public final class KeypadActivity extends Activity {
    // The direction currently held on each hat axis, so a change or a return
    // to center releases the previous one exactly once.
    private String heldHatX;
    private String heldHatY;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        setContentView(buildKeypad());
    }

    @Override
    protected void onResume() {
        super.onResume();
        // The panel is showing, so the game panel drops its on-screen deck.
        Mobile.setSecondaryKeypadActive(true);
    }

    // The handset's physical gamepad is not tied to a display, so Android
    // routes it to the top-focused window - this keypad on the second panel -
    // and never to the game on the other panel. Ebitengine reads the pad from
    // events on its own view, so once the events land here the game is deaf to
    // it. These two handlers forward the pad to the game through the same
    // bridge the on-screen keys use, and consume the events so they do not also
    // hop focus between the keypad buttons.
    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        String control = controlForKeyCode(event.getKeyCode());
        if (control != null && isFromGamepad(event)) {
            if (event.getAction() == KeyEvent.ACTION_DOWN) {
                Mobile.pressControl(control, true);
            } else if (event.getAction() == KeyEvent.ACTION_UP) {
                Mobile.pressControl(control, false);
            }
            return true;
        }
        return super.dispatchKeyEvent(event);
    }

    @Override
    public boolean onGenericMotionEvent(MotionEvent event) {
        if (isFromGamepad(event)
                && event.getActionMasked() == MotionEvent.ACTION_MOVE) {
            updateHat(
                    event.getAxisValue(MotionEvent.AXIS_HAT_X),
                    event.getAxisValue(MotionEvent.AXIS_HAT_Y)
            );
            return true;
        }
        return super.onGenericMotionEvent(event);
    }

    // Many retro handhelds report their d-pad as a hat axis rather than as key
    // events. The axis carries no press/release, so the last direction is held
    // until the value returns to center, and a change releases the old one.
    private void updateHat(float hatX, float hatY) {
        String nextX = hatX < -0.5f ? "left" : hatX > 0.5f ? "right" : null;
        String nextY = hatY < -0.5f ? "up" : hatY > 0.5f ? "down" : null;
        if (!Objects.equals(nextX, heldHatX)) {
            if (heldHatX != null) {
                Mobile.pressControl(heldHatX, false);
            }
            if (nextX != null) {
                Mobile.pressControl(nextX, true);
            }
            heldHatX = nextX;
        }
        if (!Objects.equals(nextY, heldHatY)) {
            if (heldHatY != null) {
                Mobile.pressControl(heldHatY, false);
            }
            if (nextY != null) {
                Mobile.pressControl(nextY, true);
            }
            heldHatY = nextY;
        }
    }

    private static boolean isFromGamepad(KeyEvent event) {
        return isFromGamepad(event.getSource());
    }

    private static boolean isFromGamepad(MotionEvent event) {
        return isFromGamepad(event.getSource());
    }

    private static boolean isFromGamepad(int source) {
        return (source & InputDevice.SOURCE_GAMEPAD) == InputDevice.SOURCE_GAMEPAD
                || (source & InputDevice.SOURCE_DPAD) == InputDevice.SOURCE_DPAD
                || (source & InputDevice.SOURCE_JOYSTICK) == InputDevice.SOURCE_JOYSTICK;
    }

    // The face buttons follow the frontend's default pad layout: A confirms,
    // B goes back, X and Y are the soft keys, and Start/Select are the call
    // keys. The numeric keys stay on the touch grid.
    private static String controlForKeyCode(int keyCode) {
        switch (keyCode) {
            case KeyEvent.KEYCODE_DPAD_UP:
                return "up";
            case KeyEvent.KEYCODE_DPAD_DOWN:
                return "down";
            case KeyEvent.KEYCODE_DPAD_LEFT:
                return "left";
            case KeyEvent.KEYCODE_DPAD_RIGHT:
                return "right";
            case KeyEvent.KEYCODE_DPAD_CENTER:
            case KeyEvent.KEYCODE_BUTTON_A:
                return "ok";
            case KeyEvent.KEYCODE_BUTTON_B:
                return "back";
            case KeyEvent.KEYCODE_BUTTON_X:
                return "soft-left";
            case KeyEvent.KEYCODE_BUTTON_Y:
                return "soft-right";
            case KeyEvent.KEYCODE_BUTTON_START:
                return "send";
            case KeyEvent.KEYCODE_BUTTON_SELECT:
                return "end";
            default:
                return null;
        }
    }

    @Override
    protected void onDestroy() {
        // The keypad is gone; let the game panel bring its own deck back so the
        // controls are never left unreachable.
        Mobile.setSecondaryKeypadActive(false);
        super.onDestroy();
    }

    private View buildKeypad() {
        int pad = dp(8);
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.HORIZONTAL);
        root.setBackgroundColor(Color.rgb(18, 18, 22));
        root.setPadding(pad, pad, pad, pad);
        // The physical pad is forwarded to the game, not used to move between
        // these keys, so the buttons stay out of the focus order.
        root.setDescendantFocusability(ViewGroup.FOCUS_BLOCK_DESCENDANTS);

        root.addView(buildDirectionalSide(), equalColumn());
        root.addView(buildNumericSide(), equalColumn());
        return root;
    }

    // Left column: the d-pad cross with OK at its center, then the soft keys
    // and the call/menu keys underneath.
    private View buildDirectionalSide() {
        LinearLayout column = new LinearLayout(this);
        column.setOrientation(LinearLayout.VERTICAL);

        column.addView(row(spacer(), control("▲", "up"), spacer()), weightedRow(3));
        column.addView(
                row(control("◀", "left"), control("OK", "ok"), control("▶", "right")),
                weightedRow(3)
        );
        column.addView(row(spacer(), control("▼", "down"), spacer()), weightedRow(3));
        column.addView(
                row(control("Soft L", "soft-left"), control("Menu", "menu"), control("Soft R", "soft-right")),
                weightedRow(2)
        );
        column.addView(
                row(control("Send", "send"), control("Back", "back"), control("End", "end")),
                weightedRow(2)
        );
        return column;
    }

    // Right column: the phone numeric pad, 1-9 then * 0 #.
    private View buildNumericSide() {
        LinearLayout column = new LinearLayout(this);
        column.setOrientation(LinearLayout.VERTICAL);

        column.addView(
                row(control("1", "num1"), control("2", "num2"), control("3", "num3")),
                weightedRow(1)
        );
        column.addView(
                row(control("4", "num4"), control("5", "num5"), control("6", "num6")),
                weightedRow(1)
        );
        column.addView(
                row(control("7", "num7"), control("8", "num8"), control("9", "num9")),
                weightedRow(1)
        );
        column.addView(
                row(control("*", "star"), control("0", "num0"), control("#", "hash")),
                weightedRow(1)
        );
        return column;
    }

    private LinearLayout row(View left, View middle, View right) {
        LinearLayout row = new LinearLayout(this);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.addView(left, equalCell());
        row.addView(middle, equalCell());
        row.addView(right, equalCell());
        return row;
    }

    // A control button reports its press and release so the guest sees the key
    // held for as long as the finger is down, matching the on-screen deck.
    private View control(String label, String control) {
        Button button = new Button(this);
        button.setText(label);
        button.setAllCaps(false);
        int margin = dp(3);
        button.setOnTouchListener((view, event) -> {
            switch (event.getActionMasked()) {
                case MotionEvent.ACTION_DOWN:
                    view.setPressed(true);
                    Mobile.pressControl(control, true);
                    return true;
                case MotionEvent.ACTION_UP:
                case MotionEvent.ACTION_CANCEL:
                    view.setPressed(false);
                    Mobile.pressControl(control, false);
                    return true;
                default:
                    return false;
            }
        });
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(
                0, ViewGroup.LayoutParams.MATCH_PARENT, 1f
        );
        params.setMargins(margin, margin, margin, margin);
        button.setLayoutParams(params);
        return button;
    }

    private View spacer() {
        Space space = new Space(this);
        space.setLayoutParams(new LinearLayout.LayoutParams(
                0, ViewGroup.LayoutParams.MATCH_PARENT, 1f
        ));
        return space;
    }

    private LinearLayout.LayoutParams equalColumn() {
        return new LinearLayout.LayoutParams(
                0, ViewGroup.LayoutParams.MATCH_PARENT, 1f
        );
    }

    private LinearLayout.LayoutParams equalCell() {
        return new LinearLayout.LayoutParams(
                0, ViewGroup.LayoutParams.MATCH_PARENT, 1f
        );
    }

    private LinearLayout.LayoutParams weightedRow(int weight) {
        return new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, 0, weight
        );
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}

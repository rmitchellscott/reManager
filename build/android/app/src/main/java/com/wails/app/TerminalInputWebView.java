package com.wails.app;

import android.content.Context;
import android.text.InputType;
import android.util.AttributeSet;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import android.view.inputmethod.InputMethodManager;
import android.webkit.WebView;

/**
 * The terminal's textarea already carries autocomplete="off", which Chromium
 * passes on as TYPE_TEXT_FLAG_NO_SUGGESTIONS, but Samsung Keyboard ignores that
 * flag and keeps autocorrecting and composing whole words. A visible password
 * field is the one input type it treats as literal text. Termux, ConnectBot and
 * Acode send the same value for the same reason.
 */
public class TerminalInputWebView extends WebView {
    private static final int CHARACTER_INPUT_TYPE =
            InputType.TYPE_TEXT_VARIATION_VISIBLE_PASSWORD | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS;

    private boolean terminalFocused;

    public TerminalInputWebView(Context context) {
        super(context);
    }

    public TerminalInputWebView(Context context, AttributeSet attrs) {
        super(context, attrs);
    }

    public TerminalInputWebView(Context context, AttributeSet attrs, int defStyleAttr) {
        super(context, attrs, defStyleAttr);
    }

    // Chromium restarts input on its own when focus moves between fields, but
    // that can land before the page reports the terminal's focus, so the
    // keyboard is restarted again with the new type.
    public void setTerminalFocused(boolean focused) {
        if (terminalFocused == focused) return;
        terminalFocused = focused;
        InputMethodManager inputMethodManager =
                (InputMethodManager) getContext().getSystemService(Context.INPUT_METHOD_SERVICE);
        if (inputMethodManager != null) inputMethodManager.restartInput(this);
    }

    @Override
    public InputConnection onCreateInputConnection(EditorInfo outAttrs) {
        InputConnection connection = super.onCreateInputConnection(outAttrs);
        if (terminalFocused) {
            outAttrs.inputType = CHARACTER_INPUT_TYPE;
            outAttrs.imeOptions |= EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING;
        }
        return connection;
    }
}

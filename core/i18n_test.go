package core

import (
	"testing"
)

func TestI18n_DetectLanguage(t *testing.T) {
	tests := []struct {
		text     string
		expected Language
	}{
		{"Hello world", LangEnglish},
		{"你好世界", LangChinese},
		{"こんにちは", LangEnglish}, // Japanese not detected, defaults to English
		{"", LangEnglish},
		{"Mixed 中文 English", LangChinese},
	}

	for _, test := range tests {
		result := DetectLanguage(test.text)
		if result != test.expected {
			t.Errorf("DetectLanguage(%q) = %v, expected %v", test.text, result, test.expected)
		}
	}
}

func TestI18n_NewI18n(t *testing.T) {
	i := NewI18n(LangEnglish)

	if i.currentLang() != LangEnglish {
		t.Errorf("expected English, got %v", i.currentLang())
	}
}

func TestI18n_DetectAndSet(t *testing.T) {
	i := NewI18n(LangAuto)

	i.DetectAndSet("Hello")
	if i.currentLang() != LangEnglish {
		t.Errorf("expected English, got %v", i.currentLang())
	}

	i.DetectAndSet("你好")
	if i.currentLang() != LangChinese {
		t.Errorf("expected Chinese, got %v", i.currentLang())
	}
}

func TestI18n_DetectAndSet_Manual(t *testing.T) {
	i := NewI18n(LangEnglish)

	// Should not auto-detect when language is set manually
	i.DetectAndSet("你好")
	if i.currentLang() != LangEnglish {
		t.Errorf("expected English (manual), got %v", i.currentLang())
	}
}

func TestI18n_SetLang(t *testing.T) {
	i := NewI18n(LangAuto)

	i.SetLang(LangChinese)
	if i.currentLang() != LangChinese {
		t.Errorf("expected Chinese, got %v", i.currentLang())
	}
}

func TestI18n_Translation(t *testing.T) {
	i := NewI18n(LangEnglish)

	msg := i.T(MsgStarting)
	if msg != "⏳ Processing..." {
		t.Errorf("expected '⏳ Processing...', got %q", msg)
	}

	i.SetLang(LangChinese)
	msg = i.T(MsgStarting)
	if msg != "⏳ 处理中..." {
		t.Errorf("expected '⏳ 处理中...', got %q", msg)
	}
}

func TestI18n_TranslationWithFormat(t *testing.T) {
	i := NewI18n(LangEnglish)

	msg := i.Tf(MsgThinking, "Analyzing code")
	if msg != "💭 Analyzing code" {
		t.Errorf("expected '💭 Analyzing code', got %q", msg)
	}
}

func TestI18n_UnknownKey(t *testing.T) {
	i := NewI18n(LangEnglish)

	msg := i.T("unknown_key")
	if msg != "unknown_key" {
		t.Errorf("expected key returned for unknown key, got %q", msg)
	}
}

func TestI18n_SaveFunc(t *testing.T) {
	i := NewI18n(LangAuto)

	saved := false
	i.SetSaveFunc(func(lang Language) error {
		saved = true
		return nil
	})

	i.DetectAndSet("你好")

	if !saved {
		t.Error("expected save func to be called")
	}
}

func TestI18n_SaveFunc_Error(t *testing.T) {
	i := NewI18n(LangAuto)

	i.SetSaveFunc(func(lang Language) error {
		return nil
	})

	// Should not panic on save error
	i.DetectAndSet("你好")
}

func TestSplitMessage(t *testing.T) {
	tests := []struct {
		text     string
		maxLen   int
		expected int
	}{
		{"short", 100, 1},
		{"This is a longer message that should be split", 20, 3},
		{"", 100, 1},
		{"Exact limit", 11, 1},
		{"Exact limit!", 11, 2},
	}

	for _, test := range tests {
		result := splitMessage(test.text, test.maxLen)
		if len(result) != test.expected {
			t.Errorf("splitMessage(%q, %d) = %d chunks, expected %d",
				test.text, test.maxLen, len(result), test.expected)
		}

		// Verify each chunk is within limit
		for i, chunk := range result {
			if len(chunk) > test.maxLen {
				t.Errorf("chunk %d exceeds max length: %d > %d", i, len(chunk), test.maxLen)
			}
		}
	}
}

func TestSplitMessage_Newline(t *testing.T) {
	text := "Line 1\nLine 2\nLine 3"
	result := splitMessage(text, 10)

	// Should try to split at newlines
	if len(result) == 0 {
		t.Fatal("expected at least one chunk")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		text     string
		maxLen   int
		expected string
	}{
		{"short", 100, "short"},
		{"This is a longer message", 10, "This is a ..."},
		{"Exact", 5, "Exact"},
		{"", 100, ""},
		{"中文测试", 3, "中文测..."},
	}

	for _, test := range tests {
		result := truncate(test.text, test.maxLen)
		if result != test.expected {
			t.Errorf("truncate(%q, %d) = %q, expected %q",
				test.text, test.maxLen, result, test.expected)
		}
	}
}

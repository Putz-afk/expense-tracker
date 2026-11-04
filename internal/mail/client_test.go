package mail

import "testing"

func TestFromMatches(t *testing.T) {
	const want = "bankbri@bri.co.id"

	tests := []struct {
		name string
		from string
		ok   bool
	}{
		{"display name and different case", "Bank BRI <BankBRI@bri.co.id>", true},
		{"bare address", "bankbri@bri.co.id", true},
		{"look-alike domain", "Bank BRI <bankbri@bri.co.id.evil.example>", false},
		{"look-alike user", "Bank BRI <fakebankbri@bri.co.id>", false},
		{"address hidden in the display name", `"bankbri@bri.co.id" <x@evil.example>`, false},
		{"unparsable", "not an address", false},
		{"missing", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte("From: " + tc.from + "\r\nSubject: x\r\n\r\nbody")
			if got := fromMatches(raw, want); got != tc.ok {
				t.Errorf("fromMatches(%q) = %v, want %v", tc.from, got, tc.ok)
			}
		})
	}
}

func TestFromMatchesBadMessage(t *testing.T) {
	if fromMatches([]byte("no headers here"), "bankbri@bri.co.id") {
		t.Error("fromMatches() = true for a message without headers")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Run("both set", func(t *testing.T) {
		t.Setenv(envUser, "me@gmail.com")
		t.Setenv(envPassword, "abcd efgh")
		cfg, err := ConfigFromEnv()
		if err != nil {
			t.Fatalf("ConfigFromEnv() error: %v", err)
		}
		if cfg.Username != "me@gmail.com" || cfg.Password != "abcd efgh" {
			t.Errorf("cfg = %+v", cfg)
		}
	})

	for _, missing := range []string{envUser, envPassword} {
		t.Run("missing "+missing, func(t *testing.T) {
			t.Setenv(envUser, "me@gmail.com")
			t.Setenv(envPassword, "abcd efgh")
			t.Setenv(missing, "")
			if _, err := ConfigFromEnv(); err == nil {
				t.Error("ConfigFromEnv() returned no error")
			}
		})
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	c := New(Config{Username: "u", Password: "p"})
	if c.cfg.Addr != "imap.gmail.com:993" || c.cfg.Mailbox != "INBOX" || c.cfg.From != "bankbri@bri.co.id" {
		t.Errorf("defaults not applied: %+v", c.cfg)
	}

	c = New(Config{Mailbox: "[Gmail]/All Mail"})
	if c.cfg.Mailbox != "[Gmail]/All Mail" {
		t.Errorf("explicit mailbox overwritten: %q", c.cfg.Mailbox)
	}
}

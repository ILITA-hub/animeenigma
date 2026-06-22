package eighteenanime

import "testing"

// TestEmbedHostMatching_RejectsImposters locks in the parsed-host discipline
// (finding #52): mirror selection/extraction must match on the parsed hostname
// with equality-or-strict-subdomain — never a substring of the full URL — so a
// link like https://mp4upload.evil.com/ or https://x/?turbovid=1 is rejected.
func TestEmbedHostMatching_RejectsImposters(t *testing.T) {
	legit := []string{
		"https://www.mp4upload.com/embed-abc.html",
		"https://a1.mp4upload.com:183/d/x/video.mp4",
		"https://mp4upload.com/x",
		"https://turbovidhls.com/t/abc",
	}
	for _, u := range legit {
		if extractorFor(u) == nil {
			t.Errorf("legit %q: extractorFor returned nil", u)
		}
		if serverIDFor(u) == "" {
			t.Errorf("legit %q: serverIDFor returned empty", u)
		}
	}

	imposters := []string{
		"https://mp4upload.evil.com/x",       // subdomain imposter
		"https://evil.example/mp4upload-x/",  // token in path
		"https://attacker.test/?turbovid=1",  // token in query
		"https://turbovidhls.com.evil.net/x", // suffix imposter
		"http://169.254.169.254/mp4upload",   // metadata IP, token in path
		"ftp://mp4upload.com/x",              // non-http scheme
		"notaurl::::mp4upload",               // unparseable
	}
	for _, u := range imposters {
		if got := extractorFor(u); got != nil {
			t.Errorf("imposter %q: extractorFor must be nil", u)
		}
		if got := serverIDFor(u); got != "" {
			t.Errorf("imposter %q: serverIDFor must be empty, got %q", u, got)
		}
	}

	// supportedMirrors must drop the imposter, keep the two legit mirrors, and
	// preserve mp4upload->turbovid failover order.
	mirrors := []Mirror{
		{Link: "https://turbovidhls.com/t/a"},
		{Link: "https://mp4upload.evil.com/x"}, // imposter
		{Link: "https://www.mp4upload.com/embed-a.html"},
	}
	got := supportedMirrors(mirrors)
	if len(got) != 2 {
		t.Fatalf("supportedMirrors kept %d, want 2 (imposter dropped): %+v", len(got), got)
	}
	if serverIDFor(got[0].Link) != "mp4upload" || serverIDFor(got[1].Link) != "turbovid" {
		t.Errorf("supportedMirrors failover order wrong: %+v", got)
	}
}

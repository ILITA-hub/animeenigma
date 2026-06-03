package kodikextract

import "testing"

const sampleEmbed = `
<script>
  videoInfo.type = 'seria';
  videoInfo.hash = '71dcc2d2bb2459ae1ae89f58e17cabff';
  videoInfo.id = '782423';
  var domain = "kodikplayer.com";
  var d_sign = "c0167a7b33be40af";
  var pd_sign = "c0167a7b33be40af";
  var ref = "https://kodikplayer.com/";
  var ref_sign = "a525bb4353fafa27";
</script>`

func TestParseEmbedParams(t *testing.T) {
	p, err := parseEmbedParams(sampleEmbed)
	if err != nil {
		t.Fatalf("parseEmbedParams err: %v", err)
	}
	if p.Type != "seria" || p.ID != "782423" {
		t.Fatalf("type/id wrong: %+v", p)
	}
	if p.Ref != "https://kodikplayer.com/" {
		t.Fatalf("ref wrong: %q (must not match href= attributes)", p.Ref)
	}
	if p.Domain != "kodikplayer.com" || p.DSign == "" || p.RefSign == "" {
		t.Fatalf("signed params missing: %+v", p)
	}
}

func TestParseEmbedParamsMissing(t *testing.T) {
	if _, err := parseEmbedParams("<html>nope</html>"); err == nil {
		t.Fatal("expected error for embed with no params")
	}
}

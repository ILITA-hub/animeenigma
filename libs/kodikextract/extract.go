package kodikextract

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	userAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
	baseOrigin = "https://kodikplayer.com"
	ftorPath   = "/ftor" // == atob("L2Z0b3I=") in app.player_single*.js
)

// Stream is one decoded quality variant.
type Stream struct {
	Quality int    // 360, 480, 720, ...
	M3U8URL string // absolute https URL on the Kodik CDN (solodcdn.com)
}

// Result is the decoded set of streams for an embed.
type Result struct {
	Default int      // server's default quality
	Streams []Stream // sorted ascending by Quality
	Referer string   // Referer to send to the CDN ("https://kodikplayer.com/")
}

type embedParams struct {
	Type, Hash, ID        string
	Domain, DSign, PdSign string
	Ref, RefSign          string
}

var (
	reJSVar = regexp.MustCompile(`\.(type|hash|id)\s*=\s*'([^']*)'`)
	// ref_sign BEFORE ref so the longer name wins; \b stops href= matching ref=.
	reGoVar = regexp.MustCompile(`(?m)\b(domain|d_sign|pd_sign|ref_sign|ref)\s*=\s*"([^"]*)"`)
)

func parseEmbedParams(html string) (*embedParams, error) {
	p := &embedParams{}
	for _, m := range reJSVar.FindAllStringSubmatch(html, -1) {
		switch m[1] {
		case "type":
			p.Type = m[2]
		case "hash":
			p.Hash = m[2]
		case "id":
			p.ID = m[2]
		}
	}
	// \b before the name prevents href= matching ref= and pd_sign matching d_sign.
	for _, m := range reGoVar.FindAllStringSubmatch(html, -1) {
		switch m[1] {
		case "domain":
			p.Domain = m[2]
		case "d_sign":
			p.DSign = m[2]
		case "pd_sign":
			p.PdSign = m[2]
		case "ref":
			p.Ref = m[2]
		case "ref_sign":
			p.RefSign = m[2]
		}
	}
	if p.Type == "" || p.Hash == "" || p.ID == "" || p.DSign == "" || p.RefSign == "" {
		return nil, fmt.Errorf("kodikextract: embed page missing required params")
	}
	return p, nil
}

// newClient builds an HTTP client with a cookie jar (to carry __ddg* DDoS-Guard
// cookies from the GET into the /ftor POST) and an IPv4-forced dialer
// (containers have no IPv6 egress; matches libs/videoutils).
func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp4", addr)
			},
		},
	}
}

type ftorResponse struct {
	Default int `json:"default"`
	Links   map[string][]struct {
		Src  string `json:"src"`
		Type string `json:"type"`
	} `json:"links"`
}

// Resolve fetches a Kodik embed URL and returns the decoded HLS streams.
func Resolve(ctx context.Context, embedURL string) (*Result, error) {
	if !strings.HasPrefix(embedURL, "http") {
		embedURL = "https:" + embedURL
	}
	client := newClient()

	// 1. GET embed page (carry cookies).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", baseOrigin+"/")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kodikextract: embed GET: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kodikextract: embed GET status %d", resp.StatusCode)
	}

	p, err := parseEmbedParams(string(body))
	if err != nil {
		return nil, err
	}

	// 2. POST /ftor with signed params.
	form := url.Values{
		"d":              {p.Domain},
		"d_sign":         {p.DSign},
		"pd":             {p.Domain},
		"pd_sign":        {p.PdSign},
		"ref":            {p.Ref},
		"ref_sign":       {p.RefSign},
		"bad_user":       {"false"},
		"cdn_is_working": {"true"},
		"type":           {p.Type},
		"hash":           {p.Hash},
		"id":             {p.ID},
	}
	preq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseOrigin+ftorPath, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	preq.Header.Set("User-Agent", userAgent)
	preq.Header.Set("Referer", embedURL)
	preq.Header.Set("Origin", baseOrigin)
	preq.Header.Set("X-Requested-With", "XMLHttpRequest")
	preq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	preq.Header.Set("Accept", "application/json")
	presp, err := client.Do(preq)
	if err != nil {
		return nil, fmt.Errorf("kodikextract: /ftor POST: %w", err)
	}
	pbody, _ := io.ReadAll(presp.Body)
	presp.Body.Close()
	if presp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kodikextract: /ftor status %d", presp.StatusCode)
	}

	var fr ftorResponse
	if err := json.Unmarshal(pbody, &fr); err != nil {
		return nil, fmt.Errorf("kodikextract: /ftor decode: %w", err)
	}

	// 3. Decode each quality's src.
	res := &Result{Default: fr.Default, Referer: baseOrigin + "/"}
	for qStr, links := range fr.Links {
		if len(links) == 0 {
			continue
		}
		dec, ok := DecodeSrc(links[0].Src)
		if !ok {
			continue
		}
		if strings.HasPrefix(dec, "//") {
			dec = "https:" + dec
		}
		q, _ := strconv.Atoi(qStr)
		res.Streams = append(res.Streams, Stream{Quality: q, M3U8URL: dec})
	}
	if len(res.Streams) == 0 {
		return nil, fmt.Errorf("kodikextract: no decodable streams")
	}
	sort.Slice(res.Streams, func(i, j int) bool { return res.Streams[i].Quality < res.Streams[j].Quality })
	return res, nil
}

// PickQuality returns the stream matching want, or the highest <= want, or the
// highest available. want==0 means "use Default / highest".
func (r *Result) PickQuality(want int) Stream {
	best := r.Streams[len(r.Streams)-1] // highest
	if want <= 0 {
		for _, s := range r.Streams {
			if s.Quality == r.Default {
				return s
			}
		}
		return best
	}
	var chosen Stream
	found := false
	for _, s := range r.Streams {
		if s.Quality <= want && (!found || s.Quality > chosen.Quality) {
			chosen, found = s, true
		}
	}
	if found {
		return chosen
	}
	return best
}

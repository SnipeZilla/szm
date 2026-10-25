package query

import "testing"

func TestParseStatusCS2(t *testing.T) {
	text := `Server:  Running [0.0.0.0:27015]
---------players--------
  id     time ping loss      state   rate adr name
65535 [NoChan]    0    0 challenging      0unknown ''
    2    01:23   12    0     active 786432 1.2.3.4:27005 'Snipe Zilla'
    3      BOT    0    0     active      0 'Albert'
#end
`
	ps, ok := ParseStatus(text)
	if !ok || len(ps) != 2 {
		t.Fatalf("got %v %+v", ok, ps)
	}
	if ps[0].Name != "Snipe Zilla" || ps[0].Duration != 83 || ps[0].Ping != 12 || ps[0].Bot {
		t.Errorf("human: %+v", ps[0])
	}
	if ps[1].Name != "Albert" || !ps[1].Bot {
		t.Errorf("bot: %+v", ps[1])
	}
}

func TestParseStatusEmptyCS2(t *testing.T) {
	ps, ok := ParseStatus("---------players--------\n  id     time ping loss      state   rate adr name\n#end\n")
	if !ok || len(ps) != 0 {
		t.Fatalf("got %v %+v", ok, ps)
	}
	if _, ok := ParseStatus("Unknown command"); ok {
		t.Error("not a status answer")
	}
}

func TestParseStatusSource(t *testing.T) {
	text := `hostname: SnipeZilla
# userid name uniqueid connected ping loss state adr
#      2 "Snipe" STEAM_1:0:123 1:05:12 50 0 active 1.2.3.4:27005
#      3 "Bob" BOT active
`
	ps, ok := ParseStatus(text)
	if !ok || len(ps) != 2 || ps[0].Duration != 3912 || ps[0].Ping != 50 || !ps[1].Bot {
		t.Fatalf("got %v %+v", ok, ps)
	}
}

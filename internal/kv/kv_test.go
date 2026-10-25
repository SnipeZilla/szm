package kv

import (
	"reflect"
	"strings"
	"testing"
)

const userFile = "\"GameModes.txt\"\r\n{\r\n\r\n    \"mapgroups\"\r\n    {\r\n      \"mg_bomb\"\r\n      {\r\n        \"name\"     \"mg_bomb\"\r\n        \"maps\"\r\n        {\r\n          \"de_dust2\"          \"\"\r\n          \"de_inferno\"        \"\"\r\n          \"de_mirage\"         \"\" // the best\r\n        }\r\n      }\r\n    }\r\n\r\n    \"gameTypes\" { \"classic\" { \"gameModes\" { \"competitive\" { \"maxplayers\" \"40\" } } } }\r\n}\r\n"

func TestRead(t *testing.T) {
	names, maps := MapGroups(userFile)
	if !reflect.DeepEqual(names, []string{"mg_bomb"}) || !reflect.DeepEqual(maps["mg_bomb"], []string{"de_dust2", "de_inferno", "de_mirage"}) {
		t.Fatal(names, maps)
	}
}

func TestReplaceKeepsTheRest(t *testing.T) {
	out := SetMapGroup(userFile, "MG_BOMB", []string{"de_nuke", "de_dust2"})
	_, maps := MapGroups(out)
	if !reflect.DeepEqual(maps["mg_bomb"], []string{"de_nuke", "de_dust2"}) {
		t.Fatal(out)
	}
	for _, keep := range []string{`"name"     "mg_bomb"`, `"gameTypes" { "classic" { "gameModes" { "competitive" { "maxplayers" "40" } } } }`, "\r\n          \"de_nuke\"   \"\"\r\n          \"de_dust2\"  \"\"\r\n        }"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("lost %q:\n%s", keep, out)
		}
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Fatal("line ends mixed")
	}
}

func TestNewGroupAndFile(t *testing.T) {
	out := SetMapGroup(userFile, "mg_custom", []string{"de_anubis"})
	names, maps := MapGroups(out)
	if !reflect.DeepEqual(names, []string{"mg_bomb", "mg_custom"}) || !reflect.DeepEqual(maps["mg_custom"], []string{"de_anubis"}) || len(maps["mg_bomb"]) != 3 {
		t.Fatal(names, maps, out)
	}
	// a new file
	out = SetMapGroup("", "mg_custom", []string{"de_dust2", "cs_office"})
	if names, maps := MapGroups(out); !reflect.DeepEqual(names, []string{"mg_custom"}) || len(maps["mg_custom"]) != 2 {
		t.Fatal(out)
	}
	t.Log("\n" + out)
	// a file without mapgroups
	out = SetMapGroup("\"GameModes_Server.txt\"\n{\n\t\"gameTypes\"\n\t{\n\t}\n}\n", "mg_x", []string{"de_a"})
	if _, maps := MapGroups(out); len(maps["mg_x"]) != 1 || strings.Contains(out, "\r") {
		t.Fatal(out)
	}
	t.Log("\n" + out)
	// a group without maps, a maps block on one line
	for _, in := range []string{`"G" { "mapgroups" { "mg" { "name" "mg" } } }`, `"G" { "mapgroups" { "mg" { "maps" { "de_a" "" } } } }`, `"G" { "mapgroups" { "mg" { "maps" {} } } }`} {
		out = SetMapGroup(in, "mg", []string{"de_b", "de_c"})
		if _, maps := MapGroups(out); !reflect.DeepEqual(maps["mg"], []string{"de_b", "de_c"}) {
			t.Fatal(in, "\n", out)
		}
	}
	// empty list
	out = SetMapGroup(userFile, "mg_bomb", nil)
	if _, maps := MapGroups(out); len(maps["mg_bomb"]) != 0 || !strings.Contains(out, "gameTypes") {
		t.Fatal(out)
	}
}

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

func TestDeleteMapGroup(t *testing.T) {
	in := "\"GameModes_Server.txt\"\r\n{\r\n\t\"mapgroups\"\r\n\t{\r\n\t\t\"mg_a\"\r\n\t\t{\r\n\t\t\t\"name\" \"mg_a\"\r\n\t\t\t\"maps\"\r\n\t\t\t{\r\n\t\t\t\t\"de_dust2\" \"\"\r\n\t\t\t}\r\n\t\t}\r\n\t\t\"mg_b\" // its comment stays\r\n\t\t{\r\n\t\t\t\"maps\" { \"workshop/123/de_x\" \"\" }\r\n\t\t}\r\n\t}\r\n}\r\n"
	out, ok := DeleteMapGroup(in, "MG_A")
	if !ok {
		t.Fatal("mg_a not found")
	}
	names, maps := MapGroups(out)
	if len(names) != 1 || names[0] != "mg_b" || len(maps["mg_b"]) != 1 || maps["mg_b"][0] != "workshop/123/de_x" {
		t.Fatalf("left: %v %v\n%s", names, maps, out)
	}
	if strings.Contains(out, "de_dust2") || !strings.Contains(out, "\t\t\"mg_b\" // its comment stays\r\n") {
		t.Fatalf("text:\n%s", out)
	}
	if _, ok := DeleteMapGroup(out, "mg_none"); ok {
		t.Fatal("a group that is not there was found")
	}
	out, _ = DeleteMapGroup(out, "mg_b")
	if names, _ := MapGroups(out); len(names) != 0 {
		t.Fatalf("left: %v\n%s", names, out)
	}
}

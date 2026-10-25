// Command mkres writes rsrc_windows_amd64.syso: the Windows resources of szm.exe, its icon (szm.ico) and its
// properties (Details tab: product name, product and file version, description, copyright). The version is the one
// of main.go (var version = "2.0.0"). Run in cmd/szm, after changing the version or the icon:
//
//	go generate ./cmd/szm
//
// It needs nothing but Go (no windres, no rsrc): the .syso is a small COFF object the Go linker puts in the exe.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// What Windows shows in the file's Properties, Details tab.
const (
	product     = "SnipeZilla Server Manager"
	description = "SnipeZilla Server Manager"
	company     = "SnipeZilla"
	internal    = "szm"
	original    = "szm.exe"
	langID      = 0x0409 // English (United States)
	codePage    = 1200   // Unicode
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mkres:", err)
		os.Exit(1)
	}
}

func run() error {
	src, err := os.ReadFile("main.go")
	if err != nil {
		return err
	}
	m := regexp.MustCompile(`(?m)^var version = "([^"]+)"`).FindSubmatch(src)
	if m == nil {
		return errors.New(`main.go has no var version = "x.y.z"`)
	}
	ver := string(m[1])
	nums, err := versionNumbers(ver)
	if err != nil {
		return err
	}
	ico, err := os.ReadFile("szm.ico")
	if err != nil {
		return err
	}
	images, group, err := icons(ico, 2)
	if err != nil {
		return err
	}
	var res []resource
	for k, img := range images {
		res = append(res, resource{typ: 3, id: uint32(2 + k), data: img}) // RT_ICON
	}
	res = append(res, resource{typ: 14, id: 1, data: group})                  // RT_GROUP_ICON
	res = append(res, resource{typ: 16, id: 1, data: versionInfo(ver, nums)}) // RT_VERSION
	out := coff(res)
	_ = os.Chmod("rsrc_windows_amd64.syso", 0o644) // a copy can come read-only
	if err := os.WriteFile("rsrc_windows_amd64.syso", out, 0o644); err != nil {
		return err
	}
	fmt.Printf("rsrc_windows_amd64.syso: %s %s, %d icon sizes\n", product, ver, len(images))
	return nil
}

// versionNumbers: "2.0.1" → 2, 0, 1, 0.
func versionNumbers(v string) ([4]uint16, error) {
	var n [4]uint16
	parts := strings.Split(strings.SplitN(v, "-", 2)[0], ".")
	if len(parts) > 4 {
		return n, fmt.Errorf("version %q: at most 4 numbers", v)
	}
	for k, p := range parts {
		x, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return n, fmt.Errorf("version %q: %v", v, err)
		}
		n[k] = uint16(x)
	}
	return n, nil
}

// icons splits an .ico into its images (RT_ICON) and the directory of them (RT_GROUP_ICON), numbered from first.
func icons(ico []byte, first uint16) ([][]byte, []byte, error) {
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return nil, nil, errors.New("szm.ico is not an icon")
	}
	n := int(binary.LittleEndian.Uint16(ico[4:]))
	var images [][]byte
	var g bytes.Buffer
	w := func(v any) { _ = binary.Write(&g, binary.LittleEndian, v) }
	w(uint16(0))
	w(uint16(1))
	w(uint16(n))
	for k := 0; k < n; k++ {
		e := ico[6+16*k:]
		size := binary.LittleEndian.Uint32(e[8:])
		off := binary.LittleEndian.Uint32(e[12:])
		if int(off)+int(size) > len(ico) {
			return nil, nil, errors.New("szm.ico is cut short")
		}
		images = append(images, ico[off:off+size])
		g.Write(e[:8]) // width, height, colors, reserved, planes, bit count
		w(size)
		w(first + uint16(k))
	}
	return images, g.Bytes(), nil
}

// versionInfo builds VS_VERSIONINFO: the fixed numbers, and the strings of the Details tab.
func versionInfo(ver string, n [4]uint16) []byte {
	ms := uint32(n[0])<<16 | uint32(n[1])
	ls := uint32(n[2])<<16 | uint32(n[3])
	var fixed bytes.Buffer
	for _, v := range []uint32{
		0xFEEF04BD, 0x00010000, // signature, structure version
		ms, ls, // file version
		ms, ls, // product version
		0x3F, 0, // flags mask, flags
		0x00040004, // VOS_NT_WINDOWS32
		1, 0,       // VFT_APP
		0, 0, // date
	} {
		_ = binary.Write(&fixed, binary.LittleEndian, v)
	}
	strs := [][2]string{
		{"CompanyName", company},
		{"FileDescription", description},
		{"FileVersion", ver},
		{"InternalName", internal},
		{"LegalCopyright", fmt.Sprintf("© %d %s", time.Now().Year(), company)},
		{"OriginalFilename", original},
		{"ProductName", product},
		{"ProductVersion", ver},
	}
	var table []node
	for _, s := range strs {
		table = append(table, node{key: s[0], text: true, value: utf16z(s[1])})
	}
	var tr bytes.Buffer
	_ = binary.Write(&tr, binary.LittleEndian, uint16(langID))
	_ = binary.Write(&tr, binary.LittleEndian, uint16(codePage))
	root := node{key: "VS_VERSION_INFO", value: fixed.Bytes(), children: []node{
		{key: "StringFileInfo", text: true, children: []node{
			{key: fmt.Sprintf("%04x%04x", langID, codePage), text: true, children: table},
		}},
		{key: "VarFileInfo", text: true, children: []node{
			{key: "Translation", value: tr.Bytes()},
		}},
	}}
	return root.bytes()
}

// node is a block of VS_VERSIONINFO: wLength, wValueLength, wType, its key, its value, its children (each on 4 bytes).
type node struct {
	key      string
	value    []byte
	text     bool // wType 1: the value is text (its length counted in characters)
	children []node
}

func (n node) bytes() []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 6)) // the header, set at the end
	b.Write(utf16z(n.key))
	pad4(&b)
	b.Write(n.value)
	for _, c := range n.children {
		pad4(&b)
		b.Write(c.bytes())
	}
	out := b.Bytes()
	vlen := len(n.value)
	typ := uint16(0)
	if n.text {
		typ = 1
		vlen /= 2 // characters, with the ending zero
	}
	binary.LittleEndian.PutUint16(out[0:], uint16(len(out)))
	binary.LittleEndian.PutUint16(out[2:], uint16(vlen))
	binary.LittleEndian.PutUint16(out[4:], typ)
	return out
}

func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u)+2)
	for k, c := range u {
		binary.LittleEndian.PutUint16(b[2*k:], c)
	}
	return b
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

type resource struct {
	typ, id uint32
	data    []byte
}

// coff writes the resources as a COFF object for amd64 with one .rsrc section: the resource directory (type, then
// ID, then language), the data entries (their addresses relocated by the linker), then the data.
func coff(res []resource) []byte {
	// group by type, in order (the resources come sorted by type, then ID)
	type group struct {
		typ   uint32
		items []resource
	}
	var groups []group
	for _, r := range res {
		if len(groups) == 0 || groups[len(groups)-1].typ != r.typ {
			groups = append(groups, group{typ: r.typ})
		}
		groups[len(groups)-1].items = append(groups[len(groups)-1].items, r)
	}
	dirSize := func(entries int) int { return 16 + 8*entries }
	// layout: root dir, the type dirs, the ID dirs (one language each), the data entries, the data
	size := dirSize(len(groups))
	typeAt := make([]int, len(groups))
	for k, g := range groups {
		typeAt[k] = size
		size += dirSize(len(g.items))
	}
	idAt := map[[2]int]int{}
	for k, g := range groups {
		for j := range g.items {
			idAt[[2]int{k, j}] = size
			size += dirSize(1)
		}
	}
	entryAt := map[[2]int]int{}
	for k, g := range groups {
		for j := range g.items {
			entryAt[[2]int{k, j}] = size
			size += 16
		}
	}
	dataAt := map[[2]int]int{}
	for k, g := range groups {
		for j, r := range g.items {
			size = (size + 7) &^ 7
			dataAt[[2]int{k, j}] = size
			size += len(r.data)
		}
	}
	sec := make([]byte, size)
	le := binary.LittleEndian
	dir := func(at, entries int) {
		le.PutUint16(sec[at+14:], uint16(entries)) // ID entries
	}
	dir(0, len(groups))
	var relocs []int
	for k, g := range groups {
		le.PutUint32(sec[16+8*k:], g.typ)
		le.PutUint32(sec[16+8*k+4:], uint32(typeAt[k])|0x80000000)
		dir(typeAt[k], len(g.items))
		for j, r := range g.items {
			e := typeAt[k] + 16 + 8*j
			le.PutUint32(sec[e:], r.id)
			le.PutUint32(sec[e+4:], uint32(idAt[[2]int{k, j}])|0x80000000)
			d := idAt[[2]int{k, j}]
			dir(d, 1)
			le.PutUint32(sec[d+16:], langID)
			le.PutUint32(sec[d+20:], uint32(entryAt[[2]int{k, j}]))
			de := entryAt[[2]int{k, j}]
			le.PutUint32(sec[de:], uint32(dataAt[[2]int{k, j}])) // relocated: + the section's address
			le.PutUint32(sec[de+4:], uint32(len(r.data)))
			relocs = append(relocs, de)
			copy(sec[dataAt[[2]int{k, j}]:], r.data)
		}
	}
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, le, v) }
	const fileHeader, sectionHeader = 20, 40
	relocAt := fileHeader + sectionHeader + len(sec)
	symAt := relocAt + 10*len(relocs)
	// IMAGE_FILE_HEADER
	w(uint16(0x8664)) // AMD64
	w(uint16(1))      // one section
	w(uint32(0))      // time stamp
	w(uint32(symAt))  // symbol table
	w(uint32(1))      // one symbol
	w(uint16(0))      // no optional header
	w(uint16(0x0104)) // line numbers stripped, 32-bit words
	// IMAGE_SECTION_HEADER
	b.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0))
	w(uint32(0))
	w(uint32(len(sec)))
	w(uint32(fileHeader + sectionHeader))
	w(uint32(relocAt))
	w(uint32(0))
	w(uint16(len(relocs)))
	w(uint16(0))
	w(uint32(0x40000040)) // initialized data, readable
	b.Write(sec)
	for _, r := range relocs {
		w(uint32(r)) // where
		w(uint32(0)) // symbol 0: the section
		w(uint16(3)) // IMAGE_REL_AMD64_ADDR32NB: an address relative to the image
	}
	// the symbol of the section, and an empty string table
	b.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0))
	w(int16(1))
	w(uint16(0))
	w(uint8(3)) // static
	w(uint8(0))
	w(uint32(4))
	return b.Bytes()
}

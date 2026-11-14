package tui

import (
	"strings"
	"testing"
)

func TestSpotFormEditMachine(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 120, 34
	m.beginEdit()
	m.form.setSelected(deviceFieldType)
	view := m.render()
	assertExactFrame(t, view, 120, 34)
	plain := stripANSI(view)
	for _, want := range []string{
		"MACHINES", "nas-vault", "Edit nas-vault", "field 11 of 12",
		"Name", "MAC address", "02:00:00:00:01:02", "Relay name", "Site name", "Type", "Picture file",
		"ctrl+n cycles the types", "next", "save", "cycle", "cancel",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("spotlight form missing %q:\n%s", want, plain)
		}
	}
	for _, gone := range []string{"(Ctrl+N)", "(Ctr", "◆", "MACHINE ", "field 11/12"} {
		if strings.Contains(plain, gone) {
			t.Fatalf("spotlight form still shows %q:\n%s", gone, plain)
		}
	}
}

func TestSpotFormFitsEverySize(t *testing.T) {
	for _, size := range [][2]int{{90, 24}, {100, 28}, {120, 34}, {160, 48}} {
		for _, field := range []int{0, 5, deviceFieldPicture} {
			m := spotlightModel()
			m.width, m.height = size[0], size[1]
			m.beginEdit()
			m.form.setSelected(field)
			m.form.error = "enter a valid unicast MAC address"
			view := m.render()
			assertExactFrame(t, view, size[0], size[1])
			plain := stripANSI(view)
			if !strings.Contains(plain, "enter a valid unicast MAC address") {
				t.Fatalf("%dx%d field %d: error not shown:\n%s", size[0], size[1], field, plain)
			}
			if !strings.Contains(plain, m.spotFormField(field).label) {
				t.Fatalf("%dx%d field %d: focused field scrolled away:\n%s", size[0], size[1], field, plain)
			}
		}
	}
}

func TestSpotFormLongValueKeepsCursorInView(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 100, 28
	m.beginEdit()
	m.form.setSelected(deviceFieldPicture)
	long := "/Users/example/Pictures/machines/" + strings.Repeat("x", 80) + "-tail.png"
	m.form.values[deviceFieldPicture] = long
	m.form.inputs[deviceFieldPicture].SetValue(long)
	m.form.inputs[deviceFieldPicture].CursorEnd()
	view := m.render()
	assertExactFrame(t, view, 100, 28)
	if !strings.Contains(stripANSI(view), "-tail.png") {
		t.Fatalf("cursor end of a long value is not in view:\n%s", stripANSI(view))
	}
}

func TestSpotFormOtherFormsAndFallbacks(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 120, 34
	m.beginRemoteProfile()
	plain := stripANSI(m.render())
	for _, want := range []string{"Remote for nas-vault", "Protocol", "sunshine/rdp/vnc/ssh"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("remote form missing %q:\n%s", want, plain)
		}
	}

	m = spotlightModel()
	m.width, m.height = 120, 34
	m.beginAdd()
	if plain := stripANSI(m.render()); !strings.Contains(plain, "New machine") || !strings.Contains(plain, "blank") {
		t.Fatalf("new machine form:\n%s", plain)
	}

	for name, setup := range map[string]func(*WakeModel){
		"list":   func(m *WakeModel) { m.width = 49 },
		"ascii":  func(m *WakeModel) { m.theme = NewTheme(false, true) },
		"routes": func(m *WakeModel) { m.tab = 1 },
	} {
		m := spotlightModel()
		m.beginEdit()
		setup(m)
		if m.spotFormEligible(m.width, m.height) {
			t.Fatalf("%s should keep the classic form", name)
		}
	}
}

func TestSpotFormRedactsValues(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 120, 34
	m.beginEdit()
	m.redactIdentifiers = true
	m.form.setSelected(1)
	plain := stripANSI(m.render())
	for _, secret := range []string{"198.51.100", "02:00:00", "nas-vault"} {
		if strings.Contains(plain, secret) {
			t.Fatalf("redacted form leaked %q:\n%s", secret, plain)
		}
	}
}

func BenchmarkSpotFormRender(b *testing.B) {
	m := spotlightModel()
	m.width, m.height = 160, 48
	m.beginEdit()
	b.ReportAllocs()
	for b.Loop() {
		_ = m.render()
	}
}

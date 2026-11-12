package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aklkbqx/wol/internal/netutil"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/store"
)

const (
	siteForm   wakeFormKind = "site"
	importForm wakeFormKind = "import"
	exportForm wakeFormKind = "export"
)

func siteLabels() []string {
	return []string{"Name", "IPv4 subnet (CIDR)", "Broadcast (blank=auto)", "Local interface", "Relay name (blank=direct)", "UDP port", "Probe timeout ms", "Concurrent probes"}
}
func (m *WakeModel) beginSite(edit bool) {
	vals := []string{"", "", "", "", "", "9", "2500", "4"}
	id := ""
	if edit {
		if len(m.sites) == 0 {
			return
		}
		s := m.sites[min(m.selected, len(m.sites)-1)]
		id = s.ID
		relay := ""
		for _, r := range m.relays {
			if r.ID == s.WakeRelayID {
				relay = r.Name
			}
		}
		broadcast := s.BroadcastAddress
		if computed, err := netutil.Broadcast(s.Subnet); err == nil && computed == broadcast {
			broadcast = ""
		}
		vals = []string{s.Name, s.Subnet, broadcast, s.DefaultInterface, relay, strconv.Itoa(s.DefaultPort), strconv.Itoa(s.TimeoutMS), strconv.Itoa(s.Concurrency)}
	}
	m.form = newWakeForm(siteForm, id, siteLabels(), vals, m.theme)
	m.status = "Site defaults apply when machine fields are empty. Ctrl+N cycles relay choices."
}
func (m *WakeModel) renderSites(width int) string {
	budget := m.bodyLineBudget()
	framed := width >= 44 && budget >= 7
	innerW := width
	maxRows := budget
	if framed {
		innerW = width - 4
		maxRows = max(3, budget-2)
	}

	rows := []string{m.theme.muted().Render(fitText("Sites · a add · e edit · d delete · s check settings", innerW))}
	if len(m.sites) == 0 {
		rows = append(rows, fitText("Add Home, Lab or Office to organize networks.", innerW))
	} else {
		limit := max(1, maxRows-1)
		start := max(0, min(m.selected-limit/2, max(0, len(m.sites)-limit)))
		end := min(len(m.sites), start+limit)
		for i := start; i < end; i++ {
			s := m.sites[i]
			mark := " "
			nameW := max(6, min(18, innerW/3))
			name := padVisible(fitText(m.privateField(s.Name), nameW), nameW)
			if i == m.selected {
				mark = m.theme.accent().Render(m.theme.Glyph("arrow"))
				name = m.theme.accent().Render(name)
			}
			route := "direct"
			routeStyled := m.theme.cyan().Render("direct")
			if s.WakeRelayID != "" {
				route = "relay"
				routeStyled = m.theme.amber().Render("relay")
			}
			subnet := m.privateField(s.Subnet)
			if innerW < 48 {
				rows = append(rows, fitText(fmt.Sprintf("%s %s · %s · %s", mark, m.privateField(s.Name), subnet, route), innerW))
			} else {
				rows = append(rows, fitText(fmt.Sprintf("%s %s  %s  %s", mark, name, padVisible(fitText(subnet, 18), 18), routeStyled), innerW))
			}
		}
	}
	body := strings.Join(rows, "\n")
	if framed {
		meta := fmt.Sprintf("%d sites", len(m.sites))
		return renderShearedBox(m.theme, m.theme.Glyph("diamond")+" NETWORK SITES", meta, body, width, 0, true)
	}
	return body
}
func (m *WakeModel) beginInventoryFile(export bool) {
	kind := importForm
	value := ""
	if export {
		kind = exportForm
		value = "inventory.json"
	}
	m.form = newWakeForm(kind, "", []string{"Inventory JSON path"}, []string{value}, m.theme)
	m.status = "Import merges by identity. Export creates a new private file; existing files are preserved."
}
func inventoryFile(ctx context.Context, repo *store.Store, kind wakeFormKind, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("file path is required")
	}
	if kind == exportForm {
		data, err := repo.Export(ctx)
		if err != nil {
			return err
		}
		encoded, err := store.EncodeExport(data)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(append(encoded, '\n'))
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 4<<20))
	decoder.DisallowUnknownFields()
	var data store.ExportData
	if err = decoder.Decode(&data); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	return repo.Import(ctx, data)
}

type siteCheckMsg struct{ message string }

func (m *WakeModel) checkSelectedSite() tea.Cmd {
	if len(m.sites) == 0 {
		m.status = "Add a site first."
		return nil
	}
	site := m.sites[min(m.selected, len(m.sites)-1)]
	service := m.service
	m.status = "Checking site " + site.Name + "..."
	baseCtx := m.baseContext()
	return func() tea.Msg {
		timeout := time.Duration(site.TimeoutMS) * time.Millisecond
		if timeout <= 0 {
			timeout = 2500 * time.Millisecond
		}
		ctx, cancel := context.WithTimeout(baseCtx, timeout)
		defer cancel()
		route, err := service.ResolveRoute(ctx, store.Device{SiteID: site.ID})
		if err != nil {
			return siteCheckMsg{message: site.Name + ": " + err.Error()}
		}
		if route.Kind == "relay" {
			conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(route.Relay.Address, strconv.Itoa(route.Relay.Port)))
			if err != nil {
				return siteCheckMsg{message: site.Name + ": relay TCP unavailable: " + err.Error()}
			}
			conn.Close()
			return siteCheckMsg{message: site.Name + ": relay port reachable; SSH credentials and etherwake still require verification."}
		}
		if site.Subnet != "" {
			ip, _, err := net.ParseCIDR(site.Subnet)
			if err != nil || ip == nil {
				return siteCheckMsg{message: site.Name + ": invalid site subnet."}
			}
			if netutil.LocalBroadcast(ip.String(), site.DefaultInterface) == "" {
				return siteCheckMsg{message: site.Name + ": no matching local interface; connect to this LAN or configure a relay."}
			}
		}
		return siteCheckMsg{message: site.Name + ": direct route configured to " + route.Destination.String() + "; no wake packet sent."}
	}
}

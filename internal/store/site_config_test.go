package store

import (
	"path/filepath"
	"testing"
)

func TestSitesRoundTripAndAddressSync(t *testing.T) {
	ctx := t.Context()
	repo, err := Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	relay, err := repo.CreateWakeRelay(ctx, WakeRelay{Name: "lab-router", Address: "192.168.2.1", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	site, err := repo.CreateSite(ctx, Site{Name: "Lab", Subnet: "192.168.2.0/23", WakeRelayID: relay.ID, TimeoutMS: 3000, Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	if site.BroadcastAddress != "192.168.3.255" {
		t.Fatal(site)
	}
	d, err := repo.CreateDevice(ctx, Device{Name: "pc", MACAddress: "02:00:00:00:00:01", IPAddress: "192.168.2.10", SiteID: site.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	p, err := repo.UpsertRemoteProfile(ctx, RemoteProfile{DeviceID: d.ID, Host: d.IPAddress, Protocol: "rdp", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := repo.SyncDeviceAddress(ctx, d.ID, d.IPAddress, "192.168.2.20"); err != nil || !changed {
		t.Fatalf("sync %v %v", changed, err)
	}
	p, err = repo.GetRemoteProfile(ctx, d.ID)
	if err != nil || p.Host != "192.168.2.20" {
		t.Fatalf("profile %v %v", p, err)
	}
	p.Host = "remote.local"
	if _, err = repo.UpsertRemoteProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.SyncDeviceAddress(ctx, d.ID, "192.168.2.20", "192.168.2.30"); err != nil {
		t.Fatal(err)
	}
	p, _ = repo.GetRemoteProfile(ctx, d.ID)
	if p.Host != "remote.local" {
		t.Fatal("overrode explicit host")
	}
	if changed, err := repo.SyncDeviceAddress(ctx, d.ID, "192.168.2.20", "192.168.2.40"); err != nil || changed {
		t.Fatal("stale discovery overwrote newer address")
	}
	data, err := repo.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := Open(filepath.Join(t.TempDir(), "copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if err = dst.Import(ctx, data); err != nil {
		t.Fatal(err)
	}
	sites, err := dst.ListSites(ctx)
	if err != nil || len(sites) != 1 {
		t.Fatalf("sites %v %v", sites, err)
	}
	if sites[0].Subnet != site.Subnet || sites[0].TimeoutMS != 3000 || sites[0].Concurrency != 2 || sites[0].WakeRelayID == relay.ID {
		t.Fatal(sites)
	}
	if _, err = dst.GetWakeRelay(ctx, sites[0].WakeRelayID); err != nil {
		t.Fatal(err)
	}
}

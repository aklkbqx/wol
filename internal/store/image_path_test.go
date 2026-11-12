package store

import (
	"path/filepath"
	"testing"
)

func TestDeviceTypeAndImagePathSurviveImportWithoutThem(t *testing.T) {
	repository, err := Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	ctx := t.Context()

	created, err := repository.CreateDevice(ctx, Device{
		Name:       "studio",
		MACAddress: "02:00:00:00:01:01",
		DeviceType: "laptop",
		ImagePath:  " /pictures/studio.png ",
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repository.GetDevice(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceType != "laptop" || got.ImagePath != "/pictures/studio.png" {
		t.Fatalf("created device = type %q picture %q", got.DeviceType, got.ImagePath)
	}

	// A scan or import that knows nothing about the type or picture keeps them.
	err = repository.Import(ctx, ExportData{Version: 1, Devices: []Device{{
		Name: "studio", MACAddress: "02:00:00:00:01:01", DeviceType: "unknown", Enabled: true,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err = repository.GetDevice(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceType != "laptop" || got.ImagePath != "/pictures/studio.png" {
		t.Fatalf("after import = type %q picture %q, want both kept", got.DeviceType, got.ImagePath)
	}

	// Editing can clear the picture and change the type.
	got.ImagePath = ""
	got.DeviceType = "server"
	if _, err := repository.UpdateDevice(ctx, got.ID, got); err != nil {
		t.Fatal(err)
	}
	got, err = repository.GetDevice(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceType != "server" || got.ImagePath != "" {
		t.Fatalf("after edit = type %q picture %q", got.DeviceType, got.ImagePath)
	}
}

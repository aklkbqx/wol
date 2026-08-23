package store

import "context"

// Repository is the local inventory contract used by the standalone CLI/TUI.
type Repository interface {
	Close() error

	ListSites(context.Context) ([]Site, error)
	GetSite(context.Context, string) (Site, error)
	CreateSite(context.Context, Site) (Site, error)
	UpdateSite(context.Context, string, Site) (Site, error)
	DeleteSite(context.Context, string) error

	ListDevices(context.Context) ([]Device, error)
	GetDevice(context.Context, string) (Device, error)
	CreateDevice(context.Context, Device) (Device, error)
	UpdateDevice(context.Context, string, Device) (Device, error)
	DeleteDevice(context.Context, string) error

	ListGroups(context.Context) ([]Group, error)
	GetGroup(context.Context, string) (Group, error)
	CreateGroup(context.Context, Group) (Group, error)
	UpdateGroup(context.Context, string, Group) (Group, error)
	DeleteGroup(context.Context, string) error

	RecordWakeAttempt(context.Context, WakeAttempt) (WakeAttempt, error)
	ListWakeAttempts(context.Context, int) ([]WakeAttempt, error)
	Export(context.Context) (ExportData, error)
	Import(context.Context, ExportData) error

	ListWakeRelays(context.Context) ([]WakeRelay, error)
	GetWakeRelay(context.Context, string) (WakeRelay, error)
	CreateWakeRelay(context.Context, WakeRelay) (WakeRelay, error)
	UpdateWakeRelay(context.Context, string, WakeRelay) (WakeRelay, error)
	DeleteWakeRelay(context.Context, string) error
}

var _ Repository = (*Store)(nil)

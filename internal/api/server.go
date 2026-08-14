package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/wol"
)

type Server struct {
	store         *store.Store
	webDir        string
	allowedOrigin string
	mu            sync.RWMutex
	subscribers   map[int]chan store.WakeAttempt
	nextSubID     int
}

type WakeRequest struct {
	Verify         bool `json:"verify"`
	TimeoutSeconds int  `json:"timeoutSeconds"`
	Repeat         int  `json:"repeat"`
	IntervalMS     int  `json:"intervalMs"`
}

type APIResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

func NewServer(dataStore *store.Store, webDir, allowedOrigin string) *Server {
	return &Server{
		store:         dataStore,
		webDir:        webDir,
		allowedOrigin: allowedOrigin,
		subscribers:   make(map[int]chan store.WakeAttempt),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/v1/bootstrap", s.handleBootstrap)
	mux.HandleFunc("GET /api/v1/sites", s.handleSites)
	mux.HandleFunc("POST /api/v1/sites", s.handleCreateSite)
	mux.HandleFunc("GET /api/v1/sites/{id}", s.handleSite)
	mux.HandleFunc("PATCH /api/v1/sites/{id}", s.handleUpdateSite)
	mux.HandleFunc("DELETE /api/v1/sites/{id}", s.handleDeleteSite)
	mux.HandleFunc("GET /api/v1/devices", s.handleDevices)
	mux.HandleFunc("POST /api/v1/devices", s.handleCreateDevice)
	mux.HandleFunc("GET /api/v1/devices/{id}", s.handleDevice)
	mux.HandleFunc("PATCH /api/v1/devices/{id}", s.handleUpdateDevice)
	mux.HandleFunc("DELETE /api/v1/devices/{id}", s.handleDeleteDevice)
	mux.HandleFunc("POST /api/v1/devices/{id}/wake", s.handleWakeDevice)
	mux.HandleFunc("GET /api/v1/groups", s.handleGroups)
	mux.HandleFunc("POST /api/v1/groups", s.handleCreateGroup)
	mux.HandleFunc("GET /api/v1/groups/{id}", s.handleGroup)
	mux.HandleFunc("PATCH /api/v1/groups/{id}", s.handleUpdateGroup)
	mux.HandleFunc("DELETE /api/v1/groups/{id}", s.handleDeleteGroup)
	mux.HandleFunc("POST /api/v1/groups/{id}/wake", s.handleWakeGroup)
	mux.HandleFunc("GET /api/v1/history", s.handleHistory)
	mux.HandleFunc("GET /api/v1/events", s.handleEvents)
	mux.HandleFunc("GET /api/v1/export", s.handleExport)
	mux.HandleFunc("POST /api/v1/import", s.handleImport)
	mux.HandleFunc("/", s.handleWeb)
	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("X-Frame-Options", "DENY")
		response.Header().Set("Referrer-Policy", "same-origin")
		if s.allowedOrigin != "" && request.Header.Get("Origin") == s.allowedOrigin {
			response.Header().Set("Access-Control-Allow-Origin", s.allowedOrigin)
			response.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			response.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}
		if request.Method == http.MethodOptions {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func (s *Server) handleHealth(response http.ResponseWriter, request *http.Request) {
	writeSuccess(response, map[string]any{"status": "ok", "version": "0.1.0"})
}

func (s *Server) handleBootstrap(response http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	sites, err := s.store.ListSites(ctx)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	devices, err := s.store.ListDevices(ctx)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	groups, err := s.store.ListGroups(ctx)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeSuccess(response, map[string]any{
		"version":      "0.1.0",
		"sites":        sites,
		"devices":      devices,
		"groups":       groups,
		"capabilities": map[string]bool{"sse": true, "verification": true, "importExport": true},
	})
}

func (s *Server) handleSites(response http.ResponseWriter, request *http.Request) {
	items, err := s.store.ListSites(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeSuccess(response, items)
}

func (s *Server) handleSite(response http.ResponseWriter, request *http.Request) {
	item, err := s.store.GetSite(request.Context(), request.PathValue("id"))
	if err != nil {
		writeStoreError(response, err)
		return
	}
	writeSuccess(response, item)
}

func (s *Server) handleCreateSite(response http.ResponseWriter, request *http.Request) {
	var input store.Site
	if !decodeJSON(response, request, &input) {
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		writeError(response, http.StatusBadRequest, errors.New("site name is required"))
		return
	}
	item, err := s.store.CreateSite(request.Context(), input)
	if err != nil {
		writeError(response, http.StatusConflict, err)
		return
	}
	writeSuccessStatus(response, http.StatusCreated, item)
}

func (s *Server) handleUpdateSite(response http.ResponseWriter, request *http.Request) {
	var input store.Site
	if !decodeJSON(response, request, &input) {
		return
	}
	item, err := s.store.UpdateSite(request.Context(), request.PathValue("id"), input)
	if err != nil {
		writeStoreError(response, err)
		return
	}
	writeSuccess(response, item)
}

func (s *Server) handleDeleteSite(response http.ResponseWriter, request *http.Request) {
	if err := s.store.DeleteSite(request.Context(), request.PathValue("id")); err != nil {
		writeStoreError(response, err)
		return
	}
	writeSuccess(response, map[string]bool{"deleted": true})
}

func (s *Server) handleDevices(response http.ResponseWriter, request *http.Request) {
	items, err := s.store.ListDevices(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeSuccess(response, items)
}

func (s *Server) handleDevice(response http.ResponseWriter, request *http.Request) {
	item, err := s.store.GetDevice(request.Context(), request.PathValue("id"))
	if err != nil {
		writeStoreError(response, err)
		return
	}
	writeSuccess(response, item)
}

func (s *Server) handleCreateDevice(response http.ResponseWriter, request *http.Request) {
	var input store.Device
	if !decodeJSON(response, request, &input) {
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		writeError(response, http.StatusBadRequest, errors.New("device name is required"))
		return
	}
	mac, err := wol.ParseMAC(input.MACAddress)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	input.MACAddress = mac.String()
	item, err := s.store.CreateDevice(request.Context(), input)
	if err != nil {
		writeError(response, http.StatusConflict, err)
		return
	}
	writeSuccessStatus(response, http.StatusCreated, item)
}

func (s *Server) handleUpdateDevice(response http.ResponseWriter, request *http.Request) {
	var input store.Device
	if !decodeJSON(response, request, &input) {
		return
	}
	mac, err := wol.ParseMAC(input.MACAddress)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	input.MACAddress = mac.String()
	item, err := s.store.UpdateDevice(request.Context(), request.PathValue("id"), input)
	if err != nil {
		writeStoreError(response, err)
		return
	}
	writeSuccess(response, item)
}

func (s *Server) handleDeleteDevice(response http.ResponseWriter, request *http.Request) {
	if err := s.store.DeleteDevice(request.Context(), request.PathValue("id")); err != nil {
		writeStoreError(response, err)
		return
	}
	writeSuccess(response, map[string]bool{"deleted": true})
}

func (s *Server) handleGroups(response http.ResponseWriter, request *http.Request) {
	items, err := s.store.ListGroups(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeSuccess(response, items)
}

func (s *Server) handleGroup(response http.ResponseWriter, request *http.Request) {
	item, err := s.store.GetGroup(request.Context(), request.PathValue("id"))
	if err != nil {
		writeStoreError(response, err)
		return
	}
	writeSuccess(response, item)
}

func (s *Server) handleCreateGroup(response http.ResponseWriter, request *http.Request) {
	var input store.Group
	if !decodeJSON(response, request, &input) {
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		writeError(response, http.StatusBadRequest, errors.New("group name is required"))
		return
	}
	item, err := s.store.CreateGroup(request.Context(), input)
	if err != nil {
		writeError(response, http.StatusConflict, err)
		return
	}
	writeSuccessStatus(response, http.StatusCreated, item)
}

func (s *Server) handleUpdateGroup(response http.ResponseWriter, request *http.Request) {
	var input store.Group
	if !decodeJSON(response, request, &input) {
		return
	}
	item, err := s.store.UpdateGroup(request.Context(), request.PathValue("id"), input)
	if err != nil {
		writeStoreError(response, err)
		return
	}
	writeSuccess(response, item)
}

func (s *Server) handleDeleteGroup(response http.ResponseWriter, request *http.Request) {
	if err := s.store.DeleteGroup(request.Context(), request.PathValue("id")); err != nil {
		writeStoreError(response, err)
		return
	}
	writeSuccess(response, map[string]bool{"deleted": true})
}

func (s *Server) handleWakeDevice(response http.ResponseWriter, request *http.Request) {
	var input WakeRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	attempt, err := s.wakeDevice(request.Context(), request.PathValue("id"), "device", input)
	if err != nil && attempt.PacketStatus == "failed" {
		writeError(response, http.StatusBadGateway, err)
		return
	}
	writeSuccess(response, attempt)
}

func (s *Server) handleWakeGroup(response http.ResponseWriter, request *http.Request) {
	var input WakeRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	group, err := s.store.GetGroup(request.Context(), request.PathValue("id"))
	if err != nil {
		writeStoreError(response, err)
		return
	}
	attempts := make([]store.WakeAttempt, 0, len(group.DeviceIDs))
	for _, deviceID := range group.DeviceIDs {
		attempt, wakeErr := s.wakeDevice(request.Context(), deviceID, "group", input)
		attempts = append(attempts, attempt)
		if wakeErr != nil && request.Context().Err() != nil {
			break
		}
	}
	writeSuccess(response, map[string]any{"group": group, "attempts": attempts})
}

func (s *Server) wakeDevice(ctx context.Context, deviceID, targetType string, input WakeRequest) (store.WakeAttempt, error) {
	device, err := s.store.GetDevice(ctx, deviceID)
	if err != nil {
		return store.WakeAttempt{TargetID: deviceID, PacketStatus: "failed", VerificationStatus: "not_requested", Message: err.Error()}, err
	}
	if !device.Enabled {
		err := errors.New("device is disabled")
		attempt := store.WakeAttempt{TargetType: targetType, TargetID: device.ID, TargetName: device.Name, MACAddress: device.MACAddress, PacketStatus: "failed", VerificationStatus: "not_requested", Message: err.Error()}
		s.store.RecordWakeAttempt(ctx, attempt)
		return attempt, err
	}
	destination, port, interfaceName, err := s.resolveDestination(ctx, device)
	if err != nil {
		attempt := store.WakeAttempt{TargetType: targetType, TargetID: device.ID, TargetName: device.Name, MACAddress: device.MACAddress, PacketStatus: "failed", VerificationStatus: "not_requested", Message: err.Error()}
		s.store.RecordWakeAttempt(ctx, attempt)
		return attempt, err
	}
	mac, err := wol.ParseMAC(device.MACAddress)
	if err != nil {
		attempt := store.WakeAttempt{TargetType: targetType, TargetID: device.ID, TargetName: device.Name, MACAddress: device.MACAddress, Destination: destination.String(), Port: port, PacketStatus: "failed", VerificationStatus: "not_requested", Message: err.Error()}
		s.store.RecordWakeAttempt(ctx, attempt)
		return attempt, err
	}
	repeat := input.Repeat
	if repeat == 0 {
		repeat = 3
	}
	interval := time.Duration(input.IntervalMS) * time.Millisecond
	if input.IntervalMS == 0 {
		interval = 200 * time.Millisecond
	}
	sendResult, err := wol.Send(ctx, wol.SendRequest{MAC: mac, Destination: destination, Port: port, Interface: interfaceName, Repeat: repeat, Interval: interval})
	attempt := store.WakeAttempt{TargetType: targetType, TargetID: device.ID, TargetName: device.Name, MACAddress: device.MACAddress, Destination: destination.String(), Port: port, Packets: sendResult.Packets, VerificationStatus: "not_requested"}
	if err != nil {
		attempt.PacketStatus = "failed"
		attempt.Message = err.Error()
		s.store.RecordWakeAttempt(ctx, attempt)
		s.publish(attempt)
		return attempt, err
	}
	attempt.PacketStatus = "sent"
	if input.Verify && device.IPAddress != "" && device.VerifyPort > 0 {
		attempt.VerificationStatus = "checking"
		verifyTimeout := time.Duration(input.TimeoutSeconds) * time.Second
		if input.TimeoutSeconds == 0 {
			verifyTimeout = 30 * time.Second
		}
		verifyErr := wol.WaitForTCP(ctx, device.IPAddress, device.VerifyPort, verifyTimeout)
		if verifyErr != nil {
			attempt.VerificationStatus = "timeout"
			attempt.Message = verifyErr.Error()
		} else {
			attempt.VerificationStatus = "reachable"
		}
	} else if input.Verify {
		attempt.VerificationStatus = "unavailable"
		attempt.Message = "verification requires an IP address and TCP port"
	}
	if _, err := s.store.RecordWakeAttempt(ctx, attempt); err != nil {
		return attempt, err
	}
	s.publish(attempt)
	return attempt, nil
}

func (s *Server) resolveDestination(ctx context.Context, device store.Device) (net.IP, int, string, error) {
	destination := device.BroadcastAddress
	port := device.Port
	interfaceName := device.Interface
	if device.SiteID != "" {
		site, err := s.store.GetSite(ctx, device.SiteID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, 0, "", err
		}
		if site.ID != "" {
			if destination == "" {
				destination = site.BroadcastAddress
			}
			if port == 0 {
				port = site.DefaultPort
			}
			if interfaceName == "" {
				interfaceName = site.DefaultInterface
			}
		}
	}
	if destination == "" {
		destination = "255.255.255.255"
	}
	if port == 0 {
		port = 9
	}
	ip := net.ParseIP(destination)
	if ip == nil || ip.To4() == nil {
		return nil, 0, "", fmt.Errorf("broadcast address %q is not a valid IPv4 address", destination)
	}
	return ip.To4(), port, interfaceName, nil
}

func (s *Server) handleHistory(response http.ResponseWriter, request *http.Request) {
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	items, err := s.store.ListWakeAttempts(request.Context(), limit)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeSuccess(response, items)
}

func (s *Server) handleExport(response http.ResponseWriter, request *http.Request) {
	data, err := s.store.Export(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	encoded, err := store.EncodeExport(data)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Content-Disposition", `attachment; filename="wol-export.json"`)
	response.WriteHeader(http.StatusOK)
	response.Write(encoded)
}

func (s *Server) handleImport(response http.ResponseWriter, request *http.Request) {
	var data store.ExportData
	if !decodeJSON(response, request, &data) {
		return
	}
	if err := s.store.Import(request.Context(), data); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	writeSuccess(response, map[string]bool{"imported": true})
}

func (s *Server) handleEvents(response http.ResponseWriter, request *http.Request) {
	flusher, ok := response.(http.Flusher)
	if !ok {
		writeError(response, http.StatusInternalServerError, errors.New("streaming is not supported"))
		return
	}
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("Connection", "keep-alive")
	channel, remove := s.subscribe()
	defer remove()
	response.WriteHeader(http.StatusOK)
	response.Write([]byte(": connected\n\n"))
	flusher.Flush()
	for {
		select {
		case <-request.Context().Done():
			return
		case attempt := <-channel:
			payload, _ := json.Marshal(attempt)
			fmt.Fprintf(response, "event: wake\nid: %s\ndata: %s\n\n", attempt.ID, payload)
			flusher.Flush()
		}
	}
}

func (s *Server) subscribe() (chan store.WakeAttempt, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextSubID++
	id := s.nextSubID
	channel := make(chan store.WakeAttempt, 16)
	s.subscribers[id] = channel
	return channel, func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		close(channel)
		s.mu.Unlock()
	}
}

func (s *Server) publish(attempt store.WakeAttempt) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, channel := range s.subscribers {
		select {
		case channel <- attempt:
		default:
		}
	}
}

func (s *Server) handleWeb(response http.ResponseWriter, request *http.Request) {
	if s.webDir == "" {
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		response.Write([]byte(`<!doctype html><html><body><h1>WOL server</h1><p>Build the Svelte UI and pass --web-dir ./web/build.</p></body></html>`))
		return
	}
	path := filepath.Join(s.webDir, filepath.Clean("/"+request.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(response, request, path)
		return
	}
	index := filepath.Join(s.webDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		http.NotFound(response, request)
		return
	}
	http.ServeFile(response, request, index)
}

func decodeJSON(response http.ResponseWriter, request *http.Request, destination any) bool {
	request.Body = http.MaxBytesReader(response, request.Body, 2<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(response, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return false
	}
	return true
}

func writeStoreError(response http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	}
	writeError(response, status, err)
}

func writeSuccess(response http.ResponseWriter, data any) {
	writeSuccessStatus(response, http.StatusOK, data)
}

func writeSuccessStatus(response http.ResponseWriter, status int, data any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	json.NewEncoder(response).Encode(APIResponse{Success: true, Data: data})
}

func writeError(response http.ResponseWriter, status int, err error) {
	if err == nil {
		err = errors.New("request failed")
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	json.NewEncoder(response).Encode(APIResponse{Success: false, Message: err.Error()})
}

package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"
	"golang.org/x/net/websocket"
)

// ExecHandler upgrades to WebSocket and proxies to Docker exec
func ExecHandler(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		execProxy(ws, containerID)
	}).ServeHTTP(w, r)
}

func execProxy(ws *websocket.Conn, containerID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		slog.Error("exec: docker client failed", "err", err)
		return
	}

	execConfig := types.ExecConfig{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{"/bin/sh"},
	}

	execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		slog.Error("exec: create failed", "container", containerID, "err", err)
		websocket.Message.Send(ws, "Error: "+err.Error())
		return
	}

	resp, err := cli.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{Tty: true})
	if err != nil {
		slog.Error("exec: attach failed", "err", err)
		websocket.Message.Send(ws, "Error: "+err.Error())
		return
	}
	defer resp.Close()

	// Docker stdout → WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Reader.Read(buf)
			if n > 0 {
				websocket.Message.Send(ws, string(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()

	// WebSocket → Docker stdin
	for {
		var msg string
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			if err != io.EOF {
				slog.Debug("exec: ws receive ended", "err", err)
			}
			return
		}
		resp.Conn.Write([]byte(msg))
	}
}

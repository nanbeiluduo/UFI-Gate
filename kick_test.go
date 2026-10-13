package main

import (
	"errors"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDeauthenticateOnce(t *testing.T) {
	mac := "fa:e8:2c:45:9f:0a"
	var commands []string
	err := deauthenticateOnce(mac, []string{"wrong", "right", "unused"}, func(path, command string, _ time.Time) (string, error) {
		commands = append(commands, path+":"+command)
		if path == "wrong" {
			return "FAIL", nil
		}
		if command == "STA "+mac {
			return mac + "\naid=1", nil
		}
		if command == "DEAUTHENTICATE "+mac {
			return "OK", nil
		}
		return "", errors.New("unexpected command")
	})
	if err != nil || len(commands) != 3 || commands[2] != "right:DEAUTHENTICATE "+mac {
		t.Fatal(commands, err)
	}
	for _, reply := range []string{"FAIL", "UNKNOWN COMMAND", ""} {
		err = deauthenticateOnce(mac, []string{"socket"}, func(_, command string, _ time.Time) (string, error) {
			if strings.HasPrefix(command, "STA ") {
				return mac, nil
			}
			return reply, nil
		})
		if err == nil {
			t.Fatal("failed command reported success", reply)
		}
	}
	if deauthenticateOnce(mac, nil, nil) == nil {
		t.Fatal("no control socket reported success")
	}
}

func TestHostapdDatagramProtocol(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support UNIX datagram sockets")
	}
	testGate(t)
	path := filepath.Join(dir, "hostapd")
	server, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	mac := "fa:e8:2c:45:9f:0a"
	done := make(chan error, 1)
	go func() {
		_ = server.SetDeadline(time.Now().Add(3 * time.Second))
		for _, want := range []string{"STA " + mac, "DEAUTHENTICATE " + mac} {
			buf := make([]byte, 1024)
			n, addr, e := server.ReadFromUnix(buf)
			if e != nil {
				done <- e
				return
			}
			if string(buf[:n]) != want {
				done <- errors.New("incorrect control command")
				return
			}
			reply := "OK\n"
			if strings.HasPrefix(want, "STA ") {
				reply = mac + "\naid=1\n"
			}
			if _, e = server.WriteToUnix([]byte(reply), addr); e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	if err = deauthenticateOnce(mac, []string{path}, hostapdRequest); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "kick-*"))
	if len(files) != 0 {
		t.Fatal("client socket leaked")
	}
}

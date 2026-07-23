// voicevox-supervisor makes the optional VOICEVOX workload governor-aware.
// It owns the engine child process, kills it at Critical (level 2), and keeps
// only this tiny PID 1 alive until the platform returns to Normal. This avoids
// mounting the host Docker socket while still releasing VOICEVOX CPU/RAM under
// pressure and recovering automatically afterwards.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultRedisAddr = "redis:6379"
	governorLevelKey = "ae:degradation:level"
	criticalLevel    = 2
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("voicevox-supervisor: engine command is required")
	}

	redisAddr := envOrDefault("REDIS_ADDR", defaultRedisAddr)
	pollEvery := durationOrDefault("VOICEVOX_GOVERNOR_POLL", 2*time.Second)
	stopGrace := durationOrDefault("VOICEVOX_STOP_GRACE", 10*time.Second)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	for {
		if !waitForNormal(redisAddr, pollEvery, signals) {
			return
		}

		cmd := exec.Command(os.Args[1], os.Args[2:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			log.Fatalf("voicevox-supervisor: start engine: %v", err)
		}
		log.Printf("voicevox-supervisor: engine started pid=%d", cmd.Process.Pid)

		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()
		ticker := time.NewTicker(pollEvery)

	engineLoop:
		for {
			select {
			case sig := <-signals:
				ticker.Stop()
				stopProcessGroup(cmd, exited, stopGrace)
				log.Printf("voicevox-supervisor: engine stopped after %s", sig)
				return
			case err := <-exited:
				ticker.Stop()
				if err != nil {
					log.Fatalf("voicevox-supervisor: engine exited unexpectedly: %v", err)
				}
				log.Print("voicevox-supervisor: engine exited unexpectedly")
				os.Exit(1)
			case <-ticker.C:
				level, err := readGovernorLevel(redisAddr)
				if err != nil {
					// The governor signal is fail-open across the platform: a missing
					// key or Redis outage must not create an outage by itself.
					continue
				}
				if level >= criticalLevel {
					ticker.Stop()
					log.Printf("voicevox-supervisor: governor level=%d; killing VOICEVOX engine", level)
					stopProcessGroup(cmd, exited, stopGrace)
					break engineLoop
				}
			}
		}
	}
}

// waitForNormal keeps the heavy engine down after a Critical event until the
// platform is fully back to Normal. Redis errors fail open so a missing
// governor cannot permanently suppress the feature.
func waitForNormal(redisAddr string, pollEvery time.Duration, signals <-chan os.Signal) bool {
	announced := false
	for {
		level, err := readGovernorLevel(redisAddr)
		if err != nil || level == 0 {
			return true
		}
		if !announced {
			log.Printf("voicevox-supervisor: governor level=%d; engine remains stopped until Normal", level)
			announced = true
		}
		timer := time.NewTimer(pollEvery)
		select {
		case sig := <-signals:
			timer.Stop()
			log.Printf("voicevox-supervisor: stopping after %s", sig)
			return false
		case <-timer.C:
		}
	}
}

func stopProcessGroup(cmd *exec.Cmd, exited <-chan error, grace time.Duration) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-exited:
		return
	case <-time.After(grace):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-exited
	}
}

func readGovernorLevel(addr string) (int, error) {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))

	request := fmt.Sprintf("*2\r\n$3\r\nGET\r\n$%d\r\n%s\r\n", len(governorLevelKey), governorLevelKey)
	if _, err := io.WriteString(conn, request); err != nil {
		return 0, err
	}
	value, err := readRESPBulkString(bufio.NewReader(conn))
	if err != nil {
		return 0, err
	}
	level, err := strconv.Atoi(value)
	if err != nil || level < 0 || level > criticalLevel {
		return 0, errors.New("invalid governor level")
	}
	return level, nil
}

func readRESPBulkString(reader *bufio.Reader) (string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	header = strings.TrimSuffix(strings.TrimSuffix(header, "\n"), "\r")
	if header == "$-1" {
		return "", errors.New("governor level is missing")
	}
	if !strings.HasPrefix(header, "$") {
		return "", fmt.Errorf("unexpected Redis response %q", header)
	}
	length, err := strconv.Atoi(strings.TrimPrefix(header, "$"))
	if err != nil || length < 0 || length > 16 {
		return "", errors.New("invalid Redis bulk length")
	}
	payload := make([]byte, length+2)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return "", err
	}
	if string(payload[length:]) != "\r\n" {
		return "", errors.New("invalid Redis bulk terminator")
	}
	return string(payload[:length]), nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func durationOrDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

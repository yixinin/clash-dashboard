package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	var subURL = os.Getenv("sub")
	var ua = os.Getenv("ua")
	var clash = os.Getenv("clash")

	if subURL == "" {
		fmt.Println("sub url is missing")
		os.Exit(1)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := RunSub(ctx, clash, subURL, ua)
		if err != nil {
			fmt.Println(err)
		}
		cancel()
	}()

	var ch = make(chan os.Signal, 1)

	signal.Notify(ch, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
		fmt.Println("done")
	case sig := <-ch:
		fmt.Println("received sig", sig)
	}
	os.Exit(0)
}
func RunSub(ctx context.Context, clash, subURL, ua string) error {
	tk := time.NewTimer(1 * time.Second)
	defer tk.Stop()
	var cmd *exec.Cmd
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tk.C:
			data, err := GetConfig(ctx, subURL, ua)
			if err != nil {
				fmt.Println(err)
				continue
			}
			if cmd != nil {
				cmd.Process.Kill()
			}
			err = UpdateConfig(data)
			if err != nil {
				fmt.Println(err)
			}
			cmd = exec.Command(clash)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Start()
			if err != nil {
				return err
			}
			tk.Reset(24 * time.Hour)
		}
	}
}

func UpdateConfig(data []byte) error {
	f, err := os.OpenFile("/root/.config/clash/config.yaml", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	if err == nil {
		fmt.Println("success udpate sub ~/.config/clash/config.yaml")
	}
	return err
}

func GetConfig(ctx context.Context, subURL, ua string) ([]byte, error) {
	if subURL == "" {
		return nil, errors.New("sub url is missing")
	}
	if ua == "" {
		ua = "Clash"
	}
	req, err := http.NewRequest(http.MethodGet, subURL, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header.Add("User-Agent", ua)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	data = bytes.Replace(data, []byte("allow-lan: false"), []byte("allow-lan: true"), 1)
	data = bytes.Replace(data, []byte("external-controller: '127.0.0.1:9090'"), []byte("external-controller: '0.0.0.0:9090'\nexternal-ui: dashboard"), 1)
	return data, nil
}

type ProxyInfo struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Server   string `yaml:"server"`
	Port     uint16 `yaml:"port"`
	Cipher   string `yaml:"cipher"`
	Password string `yaml:"password"`
	UDP      bool   `yaml:"udp"`
}

type ProxyGroup struct {
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Proxies  []string `yaml:"proxies"`
	URL      string   `yaml:"url,omitempty"`
	Interval int      `yaml:"interval,omitempty"`
}

var ErrMismatch = errors.New("mismatch")

type ProtocolParser interface {
	Parse(line []byte) (*ProxyInfo, error)
}

type Shadowsocks struct {
}

func (Shadowsocks) Parse(line []byte) (*ProxyInfo, error) {
	if !bytes.HasPrefix(line, []byte("ss://")) {
		return nil, ErrMismatch
	}
	val, err := url.Parse(string(line))
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(val.User.Username())
	if err != nil {
		return nil, err
	}
	pass := strings.Split(string(data), ":")
	if len(pass) < 2 {
		return nil, errors.New("decode cipher/password fail")
	}
	cipher := pass[0]
	password := pass[1]

	hostAndPort := strings.Split(val.Host, ":")
	if len(hostAndPort) < 2 {
		return nil, errors.New("decode server/port fail")
	}
	host := hostAndPort[0]
	port, err := strconv.ParseUint(hostAndPort[1], 10, 16)
	if err != nil {
		return nil, err
	}

	return &ProxyInfo{
		Name:     val.Fragment,
		Type:     "ss",
		Server:   host,
		Port:     uint16(port),
		Cipher:   cipher,
		Password: password,
		UDP:      true,
	}, nil
}

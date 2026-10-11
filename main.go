// UFI-Gate: standalone IPv4 captive portal for rooted UFI-TOOLS devices.
// No third-party runtime dependencies. See README for protocol/firmware limits.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
)

//go:embed web/*
var web embed.FS

const version = "1.0.2"

var dir string
var demo bool
var zone = time.FixedZone("Asia/Shanghai", 8*3600)
var macRE = regexp.MustCompile(`(?i)^(?:[0-9a-f]{2}:){5}[0-9a-f]{2}$`)
var ifaceRE = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,32}$`)
var domainRE = regexp.MustCompile(`^(?:\*\.)?[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)

func main() {
	flag.StringVar(&dir, "dir", "/data/ufi-gate", "状态目录")
	flag.BoolVar(&demo, "demo", false, "仅用于本机演示，不操作防火墙")
	recoverFlag := flag.Bool("recover", false, "清除本插件防火墙规则")
	cleanupOnly := flag.Bool("cleanup-rules", false, "仅清除规则，用于 supervisor 崩溃恢复")
	setup := flag.Bool("setup-token", false, "重新生成初始化 token（仅未初始化）")
	flag.Parse()
	demo = demo
	_ = recoverFlag
	_ = cleanupOnly
	_ = setup
}

package main

import (
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"github.com/tidwall/redcon"

	"github.com/prologic/bitcask"
	"github.com/prologic/bitcask/internal"
)

var (
	bind            string
	debug           bool
	version         bool
	maxDatafileSize int
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <path>\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.BoolVarP(&version, "version", "v", false, "display version information")
	flag.BoolVarP(&debug, "debug", "d", false, "enable debug logging")

	flag.StringVarP(&bind, "bind", "b", ":6379", "interface and port to bind to")

	flag.IntVar(&maxDatafileSize, "max-datafile-size", 1<<20, "maximum datafile size in bytes")
}

func main() {
	flag.Parse()

	if debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	if version {
		fmt.Printf("bitcaskd version %s", internal.FullVersion())
		os.Exit(0)
	}

	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	path := flag.Arg(0)

	db, err := bitcask.Open(path, bitcask.WithMaxDatafileSize(maxDatafileSize))
	if err != nil {
		log.WithError(err).WithField("path", path).Error("error opening database")
		os.Exit(1)
	}

	log.WithField("bind", bind).WithField("path", path).Infof("starting bitcaskd v%s", internal.FullVersion())

	err = redcon.ListenAndServe(bind,
		func(conn redcon.Conn, cmd redcon.Command) {
			switch strings.ToLower(string(cmd.Args[0])) {
			case "ping":
				conn.WriteString("PONG")
			case "quit":
				conn.WriteString("OK")
				conn.Close()
			case "set":
				if len(cmd.Args) != 3 {
					conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
					return
				}
				key := cmd.Args[1]
				value := cmd.Args[2]
				err = db.Put(key, value)
				if err != nil {
					conn.WriteString(fmt.Sprintf("ERR: %s", err))
				} else {
					conn.WriteString("OK")
				}
			case "get":
				if len(cmd.Args) != 2 {
					conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
					return
				}
				key := cmd.Args[1]
				value, err := db.Get(key)
				if err != nil {
					conn.WriteNull()
				} else {
					conn.WriteBulk(value)
				}
			case "keys":
				conn.WriteArray(db.Len())
				for key := range db.Keys() {
					conn.WriteBulk([]byte(key))
				}
			case "exists":
				if len(cmd.Args) != 2 {
					conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
					return
				}
				key := cmd.Args[1]
				if db.Has(key) {
					conn.WriteInt(1)
				} else {
					conn.WriteInt(0)
				}
			case "del":
				if len(cmd.Args) != 2 {
					conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
					return
				}
				key := cmd.Args[1]
				err := db.Delete(key)
				if err != nil {
					conn.WriteInt(0)
				} else {
					conn.WriteInt(1)
				}
			default:
				conn.WriteError("ERR unknown command '" + string(cmd.Args[0]) + "'")
			}
		},
		func(conn redcon.Conn) bool {
			return true
		},
		func(conn redcon.Conn, err error) {
		},
	)
	if err != nil {
		log.WithError(err).Fatal("oops")
	}
}

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"

	"github.com/ianschenck/envflag"
	"github.com/lstoll/wsnet"

	"github.com/tracer/tracer/server"
	"github.com/tracer/tracer/storage/postgres"
	tgrpc "github.com/tracer/tracer/transport/grpc"
	thttp "github.com/tracer/tracer/transport/http"
	"github.com/tracer/tracer/transport/zipkinhttp"
)

func main() {
	dburl := envflag.String("DATABASE_URL", "", "URL for the database")
	port := envflag.Int("PORT", 0, "Port to listen on")
	host := envflag.String("HOST", "0.0.0.0", "Host to listen on")
	envflag.Parse()
	fTemplate := flag.String("t", "", "The `directory` containing the UI code")
	flag.Parse()
	if *fTemplate == "" {
		flag.Usage()
		os.Exit(1)
	}

	flagsMissing := false
	if *dburl == "" {
		fmt.Println("Set DATABASE_URL")
		flagsMissing = true
	}
	if *port == 0 {
		fmt.Println("Set PORT")
		flagsMissing = true
	}
	if flagsMissing {
		os.Exit(1)
	}

	db, err := sql.Open("postgres", *dburl)
	if err != nil {
		log.Fatalf("Error opening database: %q", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("Error opening database: %q", err)
	}
	storage := postgres.New(&sql.DB{})

	srv := &server.Server{Storage: storage}

	mux := http.NewServeMux()

	// Static UI content
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, ".") {
			http.ServeFile(w, r, filepath.Join(*fTemplate, r.URL.Path))
			return
		}
		http.ServeFile(w, r, filepath.Join(*fTemplate, "index.html"))
	})

	// HTTP Handlers
	thttp.New(srv, "", mux)
	zipkinhttp.New(srv, "", mux)

	grpclis, wsh := wsnet.HandlerWithKeepalive(20 * time.Second)
	mux.Handle("/grpc", wsh)
	s := grpc.NewServer()
	tgrpc.NewWithGRPCServer(srv, "", s)
	go func() {
		log.Println("Starting ws-based gRPC server on /grpc")
		err := s.Serve(grpclis)
		if err != nil {
			log.Fatalf("Error starting gRPC server: %q", err)
		}
	}()

	// Run it up
	hl, err := net.Listen("tcp", *host+":"+strconv.Itoa(*port))
	if err != nil {
		log.Fatalf("Error listening: %q", err)
	}
	log.Println("Starting HTTP server")

	if err := http.Serve(hl, mux); err != nil {
		log.Fatalf("Error starting HTTP server: %q", err)
	}
	log.Println("Ending")
}

package resolved

import "strings"

// Language is a language id, as the extractors name it.
type Language string

// Language values.
const (
	LanguageGo         Language = "go"
	LanguageTypeScript Language = "typescript"
	LanguagePython     Language = "python"
	LanguageRust       Language = "rust"
	LanguageDotNet     Language = "dotnet"
)

// The package sets an only rule may name in use.
const (
	SetNetwork  = "network"
	SetProcess  = "process"
	SetDatabase = "database"
)

// libRedis is the Redis client's name in Python, TypeScript and Rust.
const libRedis = "redis"

// goTree marks a Go entry that also matches the packages below it. A Go entry
// without it is exact, so net does not take in net/url. An entry of another
// language always matches the paths below it.
const goTree = "/..."

// nodeBuiltins lists Node built-in modules with and without the node: prefix.
func nodeBuiltins(names ...string) []string {
	out := make([]string, 0, 2*len(names))
	for _, n := range names {
		out = append(out, n, "node:"+n)
	}
	return out
}

// packageSets are the shipped sets: Go, Python and TypeScript, and best effort
// for Rust and C#. The package doc lists what imports cannot show.
var packageSets = map[string]map[Language][]string{
	SetNetwork: {
		LanguageGo: {"net", "net/http/...", "net/rpc/...", "net/smtp", "google.golang.org/grpc/..."},
		LanguagePython: {
			"socket", "http.client", "http.server", "urllib.request", "ftplib", "smtplib",
			"requests", "httpx", "aiohttp", "urllib3", "grpc", "websockets",
		},
		LanguageTypeScript: append(nodeBuiltins("http", "https", "http2", "net", "dgram", "dns", "tls"),
			"axios", "node-fetch", "undici", "got", "ws", "@grpc/grpc-js"),
		LanguageRust:   {"std::net", "tokio::net", "reqwest", "hyper", "tonic"},
		LanguageDotNet: {"System.Net"},
	},
	SetProcess: {
		LanguageGo:         {"os/exec", "golang.org/x/sys/execabs"},
		LanguagePython:     {"subprocess", "multiprocessing", "pty", "pexpect", "sh", "plumbum"},
		LanguageTypeScript: append(nodeBuiltins("child_process", "cluster"), "execa", "cross-spawn", "shelljs", "zx"),
		LanguageRust:       {"std::process", "tokio::process"},
	},
	SetDatabase: {
		LanguageGo: {
			"database/sql/...", "github.com/jackc/pgx/...", "github.com/lib/pq", "github.com/go-sql-driver/mysql",
			"github.com/mattn/go-sqlite3", "modernc.org/sqlite", "go.mongodb.org/mongo-driver/...",
			"github.com/redis/go-redis/...", "gorm.io/gorm/...", "github.com/jmoiron/sqlx", "go.etcd.io/bbolt",
		},
		LanguagePython: {
			"sqlite3", "psycopg", "psycopg2", "asyncpg", "pymysql", "MySQLdb", "sqlalchemy", "pymongo", libRedis, "peewee",
		},
		LanguageTypeScript: {
			"pg", "mysql", "mysql2", "mongodb", "mongoose", "better-sqlite3", "sqlite3", libRedis, "ioredis",
			"@prisma/client", "typeorm", "sequelize", "knex", "drizzle-orm",
		},
		LanguageRust: {"sqlx", "diesel", "rusqlite", "tokio_postgres", "postgres", "mongodb", libRedis},
		LanguageDotNet: {
			"System.Data", "Microsoft.Data.SqlClient", "Microsoft.Data.Sqlite", "Microsoft.EntityFrameworkCore",
			"Npgsql", "MySql.Data", "MongoDB.Driver", "StackExchange.Redis", "Dapper",
		},
	},
}

// Uses reports whether an import of a file in language lang uses what an only
// rule names in use. A set name means the packages of that set for the
// language; a set name wins over a library of the same name. Any other use is a
// library: it matches its own import path and every path below it.
func Uses(use string, lang Language, importPath string) bool {
	set, ok := packageSets[use]
	if !ok {
		return below(lang, use, importPath)
	}
	for _, entry := range set[lang] {
		if lang == LanguageGo {
			if root, tree := strings.CutSuffix(entry, goTree); tree {
				if below(lang, root, importPath) {
					return true
				}
				continue
			}
			if entry == importPath {
				return true
			}
			continue
		}
		if below(lang, entry, importPath) {
			return true
		}
	}
	return false
}

// below reports whether importPath is root or a path below it, with the
// separator of the language.
func below(lang Language, root, importPath string) bool {
	return importPath == root || strings.HasPrefix(importPath, root+separator(lang))
}

func separator(lang Language) string {
	switch lang {
	case LanguagePython, LanguageDotNet:
		return "."
	case LanguageRust:
		return "::"
	default:
		return "/"
	}
}

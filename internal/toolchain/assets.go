package toolchain

import (
	"fmt"
	"strings"
)

const (
	archAMD64 = "amd64"
	archARM64 = "arm64"
	argvMkdir = "mkdir"
	argvTar   = "tar"
	tarGzip   = "-xzf"

	goVersion     = "1.27.3"
	nodeVersion   = "v24.22.1"
	uvVersion     = "0.13.5"
	rustupVersion = "1.29.2"
	rubyVersion   = "3.4.12"
	jdkTag        = "jdk-21.0.9%2B10"
	jdkFileTag    = "21.0.9_10"
	clojureTools  = "1.12.6.1673"

	goAMD64SHA   = "65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40"
	goARM64SHA   = "a06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49"
	nodeX64SHA   = "eb6728e77c576a35162c122c45ccadf224d89ff00abc8dae46cc2897c6002d7c"
	nodeARM64SHA = "33c37be234ba8bee041961047b19160f4d7d0f164e8e246f78bdd10feff148b3"
	uvAMD64SHA   = "f68d291c75783122dad58ab77def0e13f3233f358731db5d6d7e8715603b928f"
	uvARM64SHA   = "4bb69cf95ed13f23713bb7bc09f241896ea0c75b7c557e615a20604b82f9c403"
	rustAMD64SHA = "5be191e8c70a5007074254cb74d78dbfdd25cd5a61c27de7b8e5410122ec24f4"
	rustARM64SHA = "a9c168dbea81a84671c6b7a5c1795cb0eb699020110d107ddfca427bd797e697"

	rubyAMD64SHA = "12bbaa27dd857892daa21d5c57e1a48908c37b03540388474cf96974d03770d1"
	rubyARM64SHA = "c098862e9416148e0d5c8851ce222323ef93c23d6176e12ab5c0effc06268f49"
	jdkAMD64SHA  = "810d3773df7e0d6c4394e4e244b264c8b30e0b05a0acf542d065fd78a6b65c2f"
	jdkARM64SHA  = "edf0da4debe7cf475dbe320d174d6eed81479eb363f41e38a2efb740428c603a"
	clojureSHA   = "fe9194858e75d5af13c2e2aff92d710674d5bc5105f2b42f90a7d94d82ec023c"
)

type archInfo struct {
	goArch string
	node   string
	triple string
}

func archFromUname(raw string) (archInfo, error) {
	switch strings.TrimSpace(raw) {
	case "x86_64", archAMD64:
		return archInfo{goArch: archAMD64, node: "x64", triple: "x86_64-unknown-linux-gnu"}, nil
	case "aarch64", archARM64:
		return archInfo{goArch: archARM64, node: archARM64, triple: "aarch64-unknown-linux-gnu"}, nil
	default:
		return archInfo{}, fmt.Errorf("unsupported arch %q", strings.TrimSpace(raw))
	}
}

type toolAsset struct {
	url     string
	sha256  string
	archive string
}

func goAsset(arch archInfo) toolAsset {
	name := fmt.Sprintf("go%s.linux-%s.tar.gz", goVersion, arch.goArch)
	sha := goARM64SHA
	if arch.goArch == archAMD64 {
		sha = goAMD64SHA
	}
	return toolAsset{
		url:     "https://dl.google.com/go/" + name,
		sha256:  sha,
		archive: "/tmp/kaxa-go.tar.gz",
	}
}

func nodeAsset(arch archInfo) toolAsset {
	name := fmt.Sprintf("node-%s-linux-%s.tar.gz", nodeVersion, arch.node)
	sha := nodeARM64SHA
	if arch.node == "x64" {
		sha = nodeX64SHA
	}
	return toolAsset{
		url:     "https://nodejs.org/dist/" + nodeVersion + "/" + name,
		sha256:  sha,
		archive: "/tmp/kaxa-node.tar.gz",
	}
}

func uvAsset(arch archInfo) toolAsset {
	name := "uv-" + arch.triple + ".tar.gz"
	sha := uvARM64SHA
	if arch.goArch == archAMD64 {
		sha = uvAMD64SHA
	}
	return toolAsset{
		url:     "https://github.com/astral-sh/uv/releases/download/" + uvVersion + "/" + name,
		sha256:  sha,
		archive: "/tmp/kaxa-uv.tar.gz",
	}
}

func rustupAsset(arch archInfo) toolAsset {
	sha := rustARM64SHA
	if arch.goArch == archAMD64 {
		sha = rustAMD64SHA
	}
	return toolAsset{
		url:    "https://static.rust-lang.org/rustup/archive/" + rustupVersion + "/" + arch.triple + "/rustup-init",
		sha256: sha,
		// rustup-init picks its behavior from its own file name: under any other
		// name it acts as a rustup proxy and refuses to install.
		archive: "/tmp/rustup-init",
	}
}

// rubyAsset is a relocatable build (--enable-load-relative) from jdx/ruby.
func rubyAsset(arch archInfo) toolAsset {
	platform, sha := "arm64_linux", rubyARM64SHA
	if arch.goArch == archAMD64 {
		platform, sha = "x86_64_linux", rubyAMD64SHA
	}
	name := fmt.Sprintf("ruby-%s.%s.tar.gz", rubyVersion, platform)
	return toolAsset{
		url:     "https://github.com/jdx/ruby/releases/download/" + rubyVersion + "/" + name,
		sha256:  sha,
		archive: "/tmp/kaxa-ruby.tar.gz",
	}
}

func jdkAsset(arch archInfo) toolAsset {
	cpu, sha := "aarch64", jdkARM64SHA
	if arch.goArch == archAMD64 {
		cpu, sha = "x64", jdkAMD64SHA
	}
	name := fmt.Sprintf("OpenJDK21U-jdk_%s_linux_hotspot_%s.tar.gz", cpu, jdkFileTag)
	return toolAsset{
		url:     "https://github.com/adoptium/temurin21-binaries/releases/download/" + jdkTag + "/" + name,
		sha256:  sha,
		archive: "/tmp/kaxa-jdk.tar.gz",
	}
}

// clojureAsset is architecture independent: scripts and jars only.
func clojureAsset() toolAsset {
	return toolAsset{
		url:     "https://github.com/clojure/brew-install/releases/download/" + clojureTools + "/clojure-tools-" + clojureTools + ".tar.gz",
		sha256:  clojureSHA,
		archive: "/tmp/kaxa-clojure.tar.gz",
	}
}

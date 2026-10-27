package sprites

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

	goAMD64SHA   = "65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40"
	goARM64SHA   = "a06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49"
	nodeX64SHA   = "eb6728e77c576a35162c122c45ccadf224d89ff00abc8dae46cc2897c6002d7c"
	nodeARM64SHA = "33c37be234ba8bee041961047b19160f4d7d0f164e8e246f78bdd10feff148b3"
	uvAMD64SHA   = "f68d291c75783122dad58ab77def0e13f3233f358731db5d6d7e8715603b928f"
	uvARM64SHA   = "4bb69cf95ed13f23713bb7bc09f241896ea0c75b7c557e615a20604b82f9c403"
	rustAMD64SHA = "5be191e8c70a5007074254cb74d78dbfdd25cd5a61c27de7b8e5410122ec24f4"
	rustARM64SHA = "a9c168dbea81a84671c6b7a5c1795cb0eb699020110d107ddfca427bd797e697"
)

type spriteArch struct {
	goArch string
	node   string
	triple string
}

func archFromUname(raw string) (spriteArch, error) {
	switch strings.TrimSpace(raw) {
	case "x86_64", archAMD64:
		return spriteArch{goArch: archAMD64, node: "x64", triple: "x86_64-unknown-linux-gnu"}, nil
	case "aarch64", archARM64:
		return spriteArch{goArch: archARM64, node: archARM64, triple: "aarch64-unknown-linux-gnu"}, nil
	default:
		return spriteArch{}, fmt.Errorf("unsupported sprite arch %q", strings.TrimSpace(raw))
	}
}

type toolAsset struct {
	url     string
	sha256  string
	archive string
}

func goAsset(arch spriteArch) toolAsset {
	name := fmt.Sprintf("go%s.linux-%s.tar.gz", goVersion, arch.goArch)
	sha := goARM64SHA
	if arch.goArch == archAMD64 {
		sha = goAMD64SHA
	}
	return toolAsset{
		url:     "https://dl.google.com/go/" + name,
		sha256:  sha,
		archive: "/tmp/sbox-go.tar.gz",
	}
}

func nodeAsset(arch spriteArch) toolAsset {
	name := fmt.Sprintf("node-%s-linux-%s.tar.gz", nodeVersion, arch.node)
	sha := nodeARM64SHA
	if arch.node == "x64" {
		sha = nodeX64SHA
	}
	return toolAsset{
		url:     "https://nodejs.org/dist/" + nodeVersion + "/" + name,
		sha256:  sha,
		archive: "/tmp/sbox-node.tar.gz",
	}
}

func uvAsset(arch spriteArch) toolAsset {
	name := "uv-" + arch.triple + ".tar.gz"
	sha := uvARM64SHA
	if arch.goArch == archAMD64 {
		sha = uvAMD64SHA
	}
	return toolAsset{
		url:     "https://github.com/astral-sh/uv/releases/download/" + uvVersion + "/" + name,
		sha256:  sha,
		archive: "/tmp/sbox-uv.tar.gz",
	}
}

func rustupAsset(arch spriteArch) toolAsset {
	sha := rustARM64SHA
	if arch.goArch == archAMD64 {
		sha = rustAMD64SHA
	}
	return toolAsset{
		url:     "https://static.rust-lang.org/rustup/archive/" + rustupVersion + "/" + arch.triple + "/rustup-init",
		sha256:  sha,
		archive: "/tmp/sbox-rustup-init",
	}
}

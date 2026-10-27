package sprites

import "context"

// installClojure installs the Clojure CLI, and a JDK when java is missing.
// The brew-install tarball ships an install.sh that needs Homebrew's Ruby, so the
// same layout is built here with plain commands.
func installClojure(ctx context.Context, c sdkClient, name, token string, arch spriteArch) error {
	if _, err := runRemote(ctx, c, name, token, []string{"java", "-version"}); err != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := installJDK(ctx, c, name, token, arch); err != nil {
			return err
		}
	}
	asset := clojureAsset()
	if err := downloadVerified(ctx, c, name, token, asset); err != nil {
		return installFailed("clojure", err)
	}
	if err := extractStrip(ctx, c, name, token, asset.archive, clojurePrefix); err != nil {
		return installFailed("clojure", err)
	}
	if err := layoutClojure(ctx, c, name, token); err != nil {
		return installFailed("clojure", err)
	}
	if err := linkTool(ctx, c, name, token, clojurePrefix+"/clojure", clojureBin); err != nil {
		return installFailed("clojure", err)
	}
	return nil
}

func layoutClojure(ctx context.Context, c sdkClient, name, token string) error {
	libexec := clojurePrefix + "/libexec"
	if _, err := runRemote(ctx, c, name, token, []string{argvMkdir, "-p", libexec}); err != nil {
		return err
	}
	for _, jar := range []string{"clojure-tools-" + clojureTools + ".jar", "exec.jar"} {
		if _, err := runRemote(ctx, c, name, token, []string{"mv", clojurePrefix + "/" + jar, libexec + "/" + jar}); err != nil {
			return err
		}
	}
	script := clojurePrefix + "/clojure"
	_, err := runRemote(ctx, c, name, token, []string{"sed", "-i", "s|^install_dir=PREFIX$|install_dir=" + clojurePrefix + "|", script})
	return err
}

func installJDK(ctx context.Context, c sdkClient, name, token string, arch spriteArch) error {
	asset := jdkAsset(arch)
	if err := downloadVerified(ctx, c, name, token, asset); err != nil {
		return installFailed("jdk", err)
	}
	if err := extractStrip(ctx, c, name, token, asset.archive, jdkPrefix); err != nil {
		return installFailed("jdk", err)
	}
	if err := linkTool(ctx, c, name, token, jdkPrefix+"/bin/java", javaBin); err != nil {
		return installFailed("jdk", err)
	}
	return nil
}

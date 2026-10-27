package toolchain

import "context"

// installClojure installs the Clojure CLI, and a JDK when java is missing.
// The brew-install tarball ships an install.sh that needs Homebrew's Ruby, so the
// same layout is built here with plain commands.
func (i *installer) installClojure(ctx context.Context, arch archInfo) error {
	if _, err := i.run(ctx, []string{"java", "-version"}); err != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := i.installJDK(ctx, arch); err != nil {
			return err
		}
	}
	asset := clojureAsset()
	if err := i.downloadVerified(ctx, asset); err != nil {
		return installFailed("clojure", err)
	}
	if err := i.extractStrip(ctx, asset.archive, i.l.clojurePrefix()); err != nil {
		return installFailed("clojure", err)
	}
	if err := i.layoutClojure(ctx); err != nil {
		return installFailed("clojure", err)
	}
	if err := i.linkTool(ctx, i.l.clojurePrefix()+"/clojure", i.l.clojureBin()); err != nil {
		return installFailed("clojure", err)
	}
	return nil
}

func (i *installer) layoutClojure(ctx context.Context) error {
	libexec := i.l.clojurePrefix() + "/libexec"
	if _, err := i.run(ctx, []string{argvMkdir, "-p", libexec}); err != nil {
		return err
	}
	for _, jar := range []string{"clojure-tools-" + clojureTools + ".jar", "exec.jar"} {
		if _, err := i.run(ctx, []string{"mv", i.l.clojurePrefix() + "/" + jar, libexec + "/" + jar}); err != nil {
			return err
		}
	}
	script := i.l.clojurePrefix() + "/clojure"
	_, err := i.run(ctx, []string{"sed", "-i", "s|^install_dir=PREFIX$|install_dir=" + i.l.clojurePrefix() + "|", script})
	return err
}

func (i *installer) installJDK(ctx context.Context, arch archInfo) error {
	asset := jdkAsset(arch)
	if err := i.downloadVerified(ctx, asset); err != nil {
		return installFailed("jdk", err)
	}
	if err := i.extractStrip(ctx, asset.archive, i.l.jdkPrefix()); err != nil {
		return installFailed("jdk", err)
	}
	if err := i.linkTool(ctx, i.l.jdkPrefix()+"/bin/java", i.l.javaBin()); err != nil {
		return installFailed("jdk", err)
	}
	return nil
}

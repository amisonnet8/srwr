package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// The README of the extension is shown on the Marketplace, and vsce does not accept an SVG there, so its pictures are PNG.
// resvg (a renderer) and DejaVu (the fonts, which a machine may not have) come from npm into a temporary directory each time:
// they are tools of this step, not dependencies of the repository (the same way as @vscode/vsce, .claude/rules/distribution.md).

const renderScript = `const {Resvg}=require('@resvg/resvg-js');const fs=require('fs');const path=require('path');
const [,, inp, out, scale]=process.argv;
const dir=path.join(__dirname,'node_modules/dejavu-fonts-ttf/ttf');
const files=['DejaVuSans.ttf','DejaVuSans-Bold.ttf','DejaVuSans-Oblique.ttf','DejaVuSansMono.ttf','DejaVuSansMono-Bold.ttf'].map(f=>path.join(dir,f));
const r=new Resvg(fs.readFileSync(inp),{fitTo:{mode:'zoom',value:Number(scale||1)},font:{fontFiles:files,loadSystemFonts:false,defaultFontFamily:'DejaVu Sans',monospaceFamily:'DejaVu Sans Mono',sansSerifFamily:'DejaVu Sans'}});
fs.writeFileSync(out,r.render().asPng());
`

// pngJob is one picture to render: the SVG, the PNG, and the zoom.
type pngJob struct {
	svg, png string
	zoom     string
}

// renderPNGs renders the jobs. When npm or the network is not there it says so and leaves the old PNGs as they are (the
// pictures of the README are committed, so a machine without them can still build everything else).
func renderPNGs(jobs []pngJob, out io.Writer) error {
	tmp, err := os.MkdirTemp("", "srwr-png-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{\"name\":\"png\",\"private\":true}\n"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "render.js"), []byte(renderScript), 0o600); err != nil {
		return err
	}
	install := exec.Command("npm", "install", "--silent", "--no-audit", "--no-fund", "@resvg/resvg-js", "dejavu-fonts-ttf")
	install.Dir = tmp
	if msg, err := install.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(out, "PNG: npm install failed, the PNGs are left as they are (%v)\n%s", err, msg)
		return nil
	}
	for _, j := range jobs {
		if err := os.MkdirAll(filepath.Dir(j.png), 0o750); err != nil {
			return err
		}
		cmd := exec.Command("node", filepath.Join(tmp, "render.js"), j.svg, j.png, j.zoom) //nolint:gosec // paths made by this tool
		if msg, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("render %s: %w\n%s", j.svg, err, msg)
		}
		_, _ = fmt.Fprintln(out, "wrote", j.png)
	}
	return nil
}

// Kanon for Obsidian: a .kan document is Markdown, and a link in it is relative to its folder and written without
// ".kan", as `kan lint` reads it.
const { Plugin, TFile, normalizePath } = require("obsidian");

module.exports = class KanonPlugin extends Plugin {
  onload() {
    this.registerExtensions(["kan"], "markdown");

    const cache = this.app.metadataCache;
    const vault = this.app.vault;
    const original = cache.getFirstLinkpathDest;
    cache.getFirstLinkpathDest = function (linkpath, sourcePath) {
      return kanonLinkDest(vault, linkpath, sourcePath) ?? original.call(this, linkpath, sourcePath);
    };
    this.register(() => {
      cache.getFirstLinkpathDest = original;
    });
  }
};

// The file a link of a .kan document names: the path from the document's folder, as written or with ".kan".
function kanonLinkDest(vault, linkpath, sourcePath) {
  if (!linkpath || !sourcePath || !sourcePath.endsWith(".kan")) {
    return null;
  }
  const folder = sourcePath.includes("/") ? sourcePath.slice(0, sourcePath.lastIndexOf("/")) : "";
  const path = normalizePath(folder ? `${folder}/${linkpath}` : linkpath);
  for (const candidate of [path, `${path}.kan`]) {
    const file = vault.getAbstractFileByPath(candidate);
    if (file instanceof TFile) {
      return file;
    }
  }
  return null;
}

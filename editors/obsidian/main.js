// Kanon for Obsidian: a .kan document is Markdown, and a link in it is relative to its folder and written without
// ".kan", as `kan lint` reads it. A link shows the name of what it names: `[[../customers/hanar]]` reads "hanar".
const { Plugin, TFile, normalizePath, editorInfoField, editorLivePreviewField } = require("obsidian");
const { Decoration, ViewPlugin } = require("@codemirror/view");
const { RangeSetBuilder } = require("@codemirror/state");

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

    // Reading view, and the blocks the live preview lays out, such as tables.
    this.registerMarkdownPostProcessor((element, context) => {
      if (!context.sourcePath.endsWith(".kan")) {
        return;
      }
      for (const link of element.querySelectorAll("a.internal-link")) {
        const target = link.getAttribute("data-href");
        if (target && link.textContent === target) {
          link.textContent = linkName(target);
        }
      }
    });

    this.registerEditorExtension(linkNames);
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

// The last part of a link's path: "hanar" for "../customers/hanar".
function linkName(linkpath) {
  return linkpath.slice(linkpath.lastIndexOf("/") + 1);
}

// In the live preview, hides the folders of a link written without a label, unless the cursor is in it.
const linkNames = ViewPlugin.fromClass(
  class {
    constructor(view) {
      this.decorations = linkNameDecorations(view);
    }

    update(update) {
      if (update.docChanged || update.viewportChanged || update.selectionSet || update.transactions.length) {
        this.decorations = linkNameDecorations(update.view);
      }
    }
  },
  { decorations: (plugin) => plugin.decorations },
);

function linkNameDecorations(view) {
  const builder = new RangeSetBuilder();
  const file = view.state.field(editorInfoField, false)?.file;
  if (!file || !file.path.endsWith(".kan") || !view.state.field(editorLivePreviewField, false)) {
    return builder.finish();
  }
  const selection = view.state.selection.ranges;
  for (const { from, to } of view.visibleRanges) {
    const text = view.state.doc.sliceString(from, to);
    for (const match of text.matchAll(/(?<!!)\[\[([^\]|#\n]*\/)[^\]|\n]*\]\]/g)) {
      const start = from + match.index;
      const end = start + match[0].length;
      if (selection.some((range) => range.from <= end && range.to >= start)) {
        continue;
      }
      builder.add(start + 2, start + 2 + match[1].length, Decoration.replace({}));
    }
  }
  return builder.finish();
}

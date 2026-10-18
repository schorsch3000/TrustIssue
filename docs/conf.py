# Sphinx configuration of the Trust Issues documentation (built by Read the Docs, see .readthedocs.yaml).
# Local build: pip install -r docs/requirements.txt && sphinx-build -W -b html docs docs/_build/html

project = "Trust Issues"
author = "Dirk Heilig"
copyright = "2026 Dirk Heilig, GPL-3.0-or-later"

extensions = ["myst_parser", "sphinx_copybutton"]
myst_enable_extensions = ["colon_fence", "deflist", "attrs_block"]
myst_heading_anchors = 3

source_suffix = {".md": "markdown"}
master_doc = "index"
exclude_patterns = ["_build", "test-reports", "RELEASING.md", "Thumbs.db", ".DS_Store"]

html_theme = "furo"
html_title = "Trust Issues"
html_static_path = ["_static"]
html_css_files = ["custom.css"]
html_theme_options = {
    "source_repository": "https://github.com/schorsch3000/TrustIssue",
    "source_branch": "master",
    "source_directory": "docs/",
}

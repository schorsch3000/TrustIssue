# Sphinx configuration of the Trust Issue documentation (built by Read the Docs, see .readthedocs.yaml).
# Local build: pip install -r docs/requirements.txt && sphinx-build -W -b html docs docs/_build/html

project = "Trust Issue"
author = "Dirk Heilig"
copyright = "2026 Dirk Heilig, GPL-3.0-or-later"

extensions = ["myst_parser", "sphinx_copybutton"]
myst_enable_extensions = ["colon_fence", "deflist", "attrs_block"]
myst_heading_anchors = 3

source_suffix = {".md": "markdown"}
master_doc = "index"
exclude_patterns = ["_build", "test-reports", "RELEASING.md", "Thumbs.db", ".DS_Store"]

html_theme = "sphinx_rtd_theme"
html_title = "Trust Issue"
html_favicon = "_static/favicon.ico"
html_static_path = ["_static"]
html_css_files = ["custom.css"]
html_theme_options = {
    "navigation_depth": 3,
    "collapse_navigation": False,
}
# "Edit on GitHub" link at the top of every page
html_context = {
    "display_github": True,
    "github_user": "schorsch3000",
    "github_repo": "TrustIssue",
    "github_version": "master",
    "conf_py_path": "/docs/",
}

# Omarchy WSL

Bring the [Omarchy](https://github.com/basecamp/omarchy) theme system to WSL. 17 curated themes that switch both **Windows Terminal** and **VS Code** simultaneously — just like Omarchy does on native Linux.

![17 themes](https://img.shields.io/badge/themes-17-blue)
![WSL](https://img.shields.io/badge/platform-WSL-orange)

## Install

```bash
source <(curl -fsSL https://raw.githubusercontent.com/tvcam/omarchy-theme-wsl/main/install)
```

This will:
- Add 17 Omarchy color schemes to Windows Terminal
- Install JetBrainsMono Nerd Font (Omarchy's default)
- Set up the `theme` command in `~/bin/`
- Install a native Windows GUI theme picker

## Usage

```bash
# Interactive picker with live preview (uses fzf if available, falls back to arrow-key TUI)
theme

# Apply a theme directly
theme tokyo-night

# Launch the Windows GUI picker
theme --gui

# List available themes
theme --list
```

## Themes

| Theme | Style |
|-------|-------|
| catppuccin-latte | Light, pastel |
| catppuccin | Dark, pastel |
| ethereal | Dark, warm amber |
| everforest | Dark, muted green |
| flexoki-light | Light, ink-inspired |
| gruvbox | Dark, retro warm |
| hackerman | Dark, neon green |
| kanagawa | Dark, muted blue |
| matte-black | Dark, minimal |
| miasma | Dark, earthy |
| nord | Dark, arctic blue |
| osaka-jade | Dark, jade green |
| ristretto | Dark, coffee warm |
| rose-pine | Light, soft pink |
| tokyo-night | Dark, cool blue |
| vantablack | Dark, pure black |
| white | Light, pure white |

## What changes

When you pick a theme, **everything updates at once**:

- **Windows Terminal** — color scheme on all WSL profiles
- **VS Code** — color theme + extension auto-installed on first use
- **Windows dark/light mode** — light themes switch Windows to light mode, dark themes to dark mode
- **Title bar accent color** — matched to the theme's accent color
- **Desktop wallpaper** — each theme comes with curated wallpapers from Omarchy

## How it works

- **Windows Terminal**: Color schemes are injected into `settings.json`. The `theme` command updates the active scheme on all WSL profiles.
- **VS Code**: The `workbench.colorTheme` setting is updated in `settings.json`. Extensions are installed on demand (first use only), matching Omarchy's lazy-install approach.
- **Windows theme**: Dark/light mode and accent color are set via registry + DWM API. Wallpapers are stored locally and set via `SystemParametersInfo`.
- **GUI**: A native Windows `.exe` (built with Go + Win32 API, no dependencies) provides a clickable theme picker with live preview.
- **Fonts**: JetBrainsMono Nerd Font is installed to the user font directory and registered in the Windows registry.

## Requirements

- WSL (Ubuntu or any distro)
- Windows Terminal
- Python 3 (pre-installed on most WSL distros)
- Git
- Optional: [fzf](https://github.com/junegunn/fzf) for the best interactive experience

## Uninstall

```bash
# Remove everything
theme --uninstall

# Remove but keep theme data
theme --uninstall --keep-data
```

## Credits

Inspired by and adapted from [basecamp/omarchy](https://github.com/basecamp/omarchy). All 17 color schemes are sourced from the Omarchy project. This is an unofficial port for WSL users who want the same theme-switching experience on Windows.

## License

MIT

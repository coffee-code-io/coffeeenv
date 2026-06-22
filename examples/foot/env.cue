// Reusable Foot terminal chart.
package foot

import st "coffeeenv.dev/lib/states"

#FootTheme: {
	file: string
	data: {...}
}

#ThemeNS: {
	available: [string]: #FootTheme
	active_name: string | *"catppuccin_mocha"
	active:      #FootTheme & available[active_name]
}

#NS: {
	font: string | *"monospace:size=11"
	theme: #ThemeNS
}

#Main: {
	terminal: foot: #NS
	terminal: foot: theme: available: #CatppuccinFootThemes

	_foot: terminal.foot

	states: {
		"foot-install": st.#ShellState & {
			run:    "sudo apt-get update && sudo apt-get install -y foot"
			unless: "command -v foot >/dev/null 2>&1"
			order: 20
		}
		"foot-theme": st.#FileState & {
			path:   "$HOME/.config/foot/themes/\(_foot.theme.active.file)"
			format: "toml"
			data:   _foot.theme.active.data
			order:  50
		}
		"foot-config": st.#FileState & {
			path: "$HOME/.config/foot/foot.ini"
			content: """
				[main]
				font=\(_foot.font)
				include=~/.config/foot/themes/\(_foot.theme.active.file)
				"""
			order: 51
		}
	}
}

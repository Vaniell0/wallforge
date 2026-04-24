package main

import (
	"fmt"
	"os"
)

// cmdCompletion emits a static completion script for the requested shell.
// The output goes to stdout so users can redirect it wherever their shell
// likes completion files (source-able from shell rc or dropped into a
// fpath / completions.d dir).
func cmdCompletion(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("completion: expected one of bash|zsh|fish, got %d args", len(args))
	}
	switch args[0] {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		return fmt.Errorf("completion: unsupported shell %q (want bash, zsh, or fish)", args[0])
	}
	_ = os.Stdout.Sync()
	return nil
}

// The completions are handwritten rather than generated from flag
// definitions — the CLI is small enough that duplication costs less
// than wiring up a completion framework (cobra/pflag) that would pull
// in a dozen transitive deps.

const bashCompletion = `# wallforge bash completion
_wallforge() {
    local cur prev cmds
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    cmds="apply shuffle serve resume workspace watchdog list config stop completion version help"

    if [[ $COMP_CWORD -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${cmds}" -- "${cur}") )
        return
    fi

    case "${COMP_WORDS[1]}" in
        apply)
            # File path or numeric ID — let bash do its file thing.
            COMPREPLY=( $(compgen -f -- "${cur}") )
            ;;
        shuffle)
            if [[ "${cur}" == --type=* ]]; then
                COMPREPLY=( $(compgen -W "image video scene web" -- "${cur#--type=}") )
                COMPREPLY=( "${COMPREPLY[@]/#/--type=}" )
                return
            fi
            COMPREPLY=( $(compgen -W "--interval= --type= --random=true --random=false" -- "${cur}") )
            ;;
        serve)
            COMPREPLY=( $(compgen -W "--addr=" -- "${cur}") )
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") )
            ;;
        workspace)
            if [[ $COMP_CWORD -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "bind unbind list daemon" -- "${cur}") )
            fi
            ;;
    esac
}
complete -F _wallforge wallforge
`

const zshCompletion = `#compdef wallforge
# wallforge zsh completion

_wallforge() {
    local -a commands
    commands=(
        'apply:set wallpaper from a path or Steam Workshop ID'
        'shuffle:cycle through a playlist at an interval'
        'serve:start the local web-UI'
        'resume:re-apply the last-used wallpaper'
        'list:list subscribed WE items'
        'config:show config path and effective values'
        'stop:stop running backends'
        'completion:print shell completion script'
        'version:print version'
        'help:show usage'
    )
    if (( CURRENT == 2 )); then
        _describe -t commands 'wallforge command' commands
        return
    fi
    case "${words[2]}" in
        apply)
            _files
            ;;
        shuffle)
            _arguments \
                '--interval=[time between changes]:duration:' \
                '--type=[WE type filter]:(image video scene web)' \
                '--random=[shuffle vs cycle]:(true false)'
            ;;
        serve)
            _arguments '--addr=[bind address]:host\:port:'
            ;;
        completion)
            _values 'shell' bash zsh fish
            ;;
    esac
}
_wallforge "$@"
`

const fishCompletion = `# wallforge fish completion
complete -c wallforge -n '__fish_use_subcommand' -a apply      -d 'set wallpaper'
complete -c wallforge -n '__fish_use_subcommand' -a shuffle    -d 'cycle through a playlist'
complete -c wallforge -n '__fish_use_subcommand' -a serve      -d 'start local web-UI'
complete -c wallforge -n '__fish_use_subcommand' -a resume     -d 're-apply last wallpaper'
complete -c wallforge -n '__fish_use_subcommand' -a list       -d 'list WE subscriptions'
complete -c wallforge -n '__fish_use_subcommand' -a config     -d 'show config'
complete -c wallforge -n '__fish_use_subcommand' -a stop       -d 'stop running backends'
complete -c wallforge -n '__fish_use_subcommand' -a completion -d 'print completion script'
complete -c wallforge -n '__fish_use_subcommand' -a version    -d 'print version'

complete -c wallforge -n '__fish_seen_subcommand_from apply'   -F
complete -c wallforge -n '__fish_seen_subcommand_from shuffle' -l interval -d 'duration between changes'
complete -c wallforge -n '__fish_seen_subcommand_from shuffle' -l type     -d 'WE type filter' -xa 'image video scene web'
complete -c wallforge -n '__fish_seen_subcommand_from shuffle' -l random   -d 'shuffle vs cycle' -xa 'true false'
complete -c wallforge -n '__fish_seen_subcommand_from serve'   -l addr     -d 'bind address'
complete -c wallforge -n '__fish_seen_subcommand_from completion' -xa 'bash zsh fish'
`

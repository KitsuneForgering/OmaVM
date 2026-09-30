# Terminal da Box: o que o Ptyxis faz

O Ptyxis é o terminal padrão do Fedora Workstation, do RHEL e do Ubuntu,
feito para containers: descobre Podman, Toolbox e Distrobox sozinho (via
`ptyxis-agent`), abre uma sessão direto num container e mantém o contexto
(container ativo e diretório) em abas novas. Três ideias dele cabem no
terminal embutido das Boxes.

## 1. Avisar antes de fechar com um processo rodando

O Ptyxis acompanha o processo em primeiro plano de cada sessão. Na OmaVM,
fechar a janela do terminal (ou Ctrl+Alt+Q) no meio de um `dnf upgrade`
dentro da Box mata o processo sem aviso.

**O jeito óbvio não funciona aqui** (conferido em 2026-09-28): olhar o
grupo de processos em primeiro plano do nosso PTY (`tcgetpgrp`) veria
sempre o `podman`, porque o `distrobox enter` roda `podman exec
--interactive --tty` (`/usr/bin/distrobox-enter`, linhas 379 e 402), que
cria **outro** PTY dentro do container. O `dnf` fica atrás desse segundo
terminal, invisível do lado da OmaVM. É por isso que o Ptyxis tem um
agente (`ptyxis-agent`) e que o problema dele é outro.

Caminhos possíveis, todos com custo:
- perguntar ao container na hora de fechar (`podman top` ou `ps` via
  `distrobox enter -- ps`), o que custa um `exec` e pode travar se o
  engine estiver lento, justamente quando a pessoa quer fechar;
- sinais do shell da Box via OSC 133/OSC 7 (marcadores de prompt que
  bash/zsh/fish emitem com integração de shell): se o último evento foi
  "comando começou" sem "comando terminou", há algo rodando. Não exige
  processo extra, mas depende da configuração do shell na Box.

Sem uma dessas, perguntar sempre ao fechar seria só incômodo. Precisa de
decisão antes de implementar.

## 2. Cor do ambiente na janela

O Ptyxis muda a cor da barra conforme a sessão (container, root). A OmaVM
já tem cor por ambiente (Color tags). Mostrar essa cor no terminal (uma
faixa fina ou o fundo da barra de status) deixa claro em qual Box se está,
o que também reforça que aquilo **não** é o host (Security Model: uma Box
compartilha o kernel e o `$HOME`, e isso não deve ser escondido).

## 3. Abrir outra sessão no mesmo contexto

O Ptyxis abre abas novas no mesmo container e diretório. Um "Nova janela
nesta Box" que já entre no diretório atual da sessão (lido de
`/proc/<pid>/cwd` do processo em primeiro plano, via `distrobox enter`)
evita repetir o `cd`. Janelas separadas, não abas: o `CLAUDE.md` mantém o
terminal simples e a organização de janelas é do Hyprland.

Já feito em 2026-09-28, na linha do Ptyxis: a janela não fecha mais quando
a sessão termina com erro; mostra o motivo e oferece "Try Again".

## Fontes

- [ptyxis(1) — a container-oriented terminal](https://www.mankier.com/1/ptyxis)
- [7 Features I Like in Ptyxis – It's FOSS](https://itsfoss.com/ptyxis-terminal-features/)
- [Ptyxis 49.3 with APX containers support – UbuntuHandbook](https://ubuntuhandbook.org/index.php/2026/01/ptyxis-49-3-apx-containers/)

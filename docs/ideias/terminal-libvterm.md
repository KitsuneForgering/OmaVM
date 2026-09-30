# Terminal da Box sobre libvterm

## A premissa que mudou

O `CLAUDE.md` explica o emulador próprio (`gui/terminal.*`) pela licença:
"viewers de terminal maduros como QTermWidget são GPL" e o projeto é MIT.
Mas existe um motor maduro com licença compatível:

- **libvterm** (MIT), a biblioteca de emulação que o Neovim usa, mantida em
  `neovim/libvterm`. Já está instalada neste host (0.3.3, dependência do
  `neovim`, com `pkg-config vterm`).
- O **Qt Creator** embute o libvterm 0.3.3 no terminal integrado dele, o
  que mostra que a combinação Qt + libvterm é usada em produção.

Isso segue a regra "integre, não reimplemente" (Backend Rules): o
emulador próprio já precisou de correções que o libvterm resolve há anos,
como caracteres largos, acentos combinantes e metade de caractere largo
sobrescrita (corrigidos aqui em 2026-09-28).

## O que mudaria

- `TerminalSession` continua dono do PTY (`forkpty` + `omavm open NAME`);
  só o parser e o modelo de tela passam para o libvterm (`vterm_input_write`,
  callbacks de `damage`/`movecursor`/`settermprop`/`sb_pushline`).
- `TerminalView` continua desenhando com QPainter a partir das células do
  libvterm (`vterm_screen_get_cell`), que já trazem largura, atributos e
  cores; a paleta continua vindo do `colors.toml`.
- Ganhos: cobertura bem maior de sequências (mouse reporting, modos DEC,
  OSC), reflow testado, menos código próprio para manter.

## Custos e riscos

- Nova dependência de sistema (`libvterm`, `libvterm-dev` no CI).
- O scrollback é responsabilidade do cliente (callbacks `sb_pushline` e
  `sb_popline`); o libvterm não faz reflow do histórico ao alargar a
  janela.
- É uma decisão de arquitetura: trocar o emulador precisa de acordo antes,
  não de uma troca incidental numa rodada de bugs (Rules for AI Agents #9).
- Os testes atuais (`make test-terminal`) viram testes de integração com o
  libvterm; os de PTY real continuam valendo.

Alternativa registrada: o **kvit-term** (Qt Quick, sobre libvterm) é
MPL-2.0. O copyleft dele é por arquivo, compatível com um projeto MIT desde
que os arquivos dele fiquem separados, mas é uma dependência bem maior que
o libvterm puro.

## Fontes

- [neovim/libvterm](https://github.com/neovim/libvterm)
- [libvterm 0.3.3 – Qt Creator Documentation (attribution)](https://doc.qt.io/qtcreator/qtcreator-attribution-libvterm.html)
- [kvit-term](https://github.com/kvit-s/kvit-term)
- [sff: a terminal using libvterm and Qt](https://github.com/jsbronder/sff)

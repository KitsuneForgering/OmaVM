# Acessibilidade: leitor de tela e teclado

## Situação

O Qt 6 expõe a interface ao **Orca** (o leitor de tela do desktop Linux)
pela ponte AT-SPI 2, mas só o que cada controle declara: nome, papel e
estado. A QML da OmaVM tem poucas anotações `Accessible.*` (duas no card,
duas em Settings, três no Main) e nenhuma navegação de teclado explícita
(`KeyNavigation`, `activeFocusOnTab`).

Corrigido em 2026-09-28: os botões de cor em Settings diziam só "red,
button"… sem informar qual estava selecionado, porque a seleção era apenas
uma borda mais grossa. Agora eles declaram `Accessible.checkable/checked`
(teste `test_selectedColorIsAnnouncedToScreenReaders`).

## O que revisar

- **Card do ambiente:** a bolinha de status comunica só por cor; o texto ao
  lado já diz o estado, mas o leitor não sabe que os dois se relacionam.
  Nome e estado do card deveriam formar um único `Accessible.name`
  ("Fedora, Box, rodando").
- **Botões só com ícone** (novo ambiente, atualizar, menu "⋯") precisam de
  `Accessible.name` em todos; hoje só parte tem.
- **Teclado:** do topo da janela até a última ação do último card, tudo
  alcançável com Tab, e o menu do card abrindo com Enter/Espaço. Os
  diálogos já fecham com Esc; conferir o foco inicial de cada um.
- **Viewer e terminal:** capturam o teclado de propósito (tudo vai para o
  guest ou para o shell da Box). O atalho de saída (Ctrl+Alt+Q) precisa
  estar anunciado no título ou numa dica, porque sem ele a pessoa fica
  presa na janela.
- **Wayland:** atalhos globais do Orca ainda dependem de protocolos em
  discussão no Wayland; testar numa sessão Hyprland real antes de prometer
  suporte.

## Como testar

O `make test-qml` já roda os componentes reais com um backend falso: dá
para verificar `Accessible.name`/`role`/`checked` e a ordem de foco por
Tab sem leitor de tela. O Accerciser (inspetor AT-SPI) serve para conferir
o que o Orca realmente recebe numa sessão real.

## Fontes

- [Accessibility – freedesktop.org](https://www.freedesktop.org/wiki/Accessibility/)
- [Wayland accessibility notes](https://github.com/splondike/wayland-accessibility-notes/blob/main/README.md)
- [Keyboard issues with Orca on Wayland – wayland-devel](https://lists.freedesktop.org/hyperkitty/list/wayland-devel@lists.freedesktop.org/thread/T7V56JTKGTR7GTNQF4H2KWDQC6MUIA5J/)
- [Keyboard navigation and screen-reader support in Qt Quick (exemplo de checklist)](https://github.com/altqx/hikari/issues/18)

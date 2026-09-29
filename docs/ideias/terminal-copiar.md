# Copiar do terminal da Box

## Situação

O terminal embutido das Boxes cola (Shift+Insert, Ctrl+Shift+V, bracketed
paste, desde 2026-09-28) mas **não copia**: não há seleção com o mouse nem
suporte a programas que copiam por conta própria. Ver também o item de P0
do `docs/TODO.md` sobre Super+C, que depende de o terminal ter algo para
copiar com Ctrl+Insert.

## Seleção com o mouse

O mínimo esperado de um terminal: arrastar seleciona, Ctrl+Insert (o que o
Omarchy manda para terminais no Super+C) e Ctrl+Shift+C copiam, clique duplo
seleciona uma palavra. A seleção precisa respeitar as células largas (uma
seleção não pode cortar um CJK ao meio) e o scrollback.

## OSC 52: programas copiando pelo terminal

O OSC 52 é uma sequência de escape com a qual o programa pede ao terminal
para pôr texto na área de transferência do sistema. tmux e Neovim (que tem
um provedor OSC 52 embutido) usam isso, e é o que faz "copiar no vim"
chegar ao Omarchy.

Cuidados:
- Só **escrita**. Ler a área de transferência via OSC 52 deixa qualquer
  comando dentro da Box ler o que o usuário copiou no host; o xterm deixa o
  OSC 52 desligado por padrão por esse motivo, e o suporte a leitura é
  instável mesmo em tmux.
- Mesmo a escrita vem de código que roda na Box: um comando não confiável
  pode trocar o conteúdo da área de transferência. Uma Box compartilha o
  kernel e o `$HOME` com o host (Security Model), então isso não abre um
  canal novo, mas a opção deve ter limite de tamanho e poder ser desligada,
  como o clipboard da Machine já é por padrão.

## Fontes

- [tmux wiki: Clipboard](https://github.com/tmux/tmux/wiki/Clipboard)
- [On tmux OSC-52 support](https://kalnytskyi.com/posts/on-tmux-osc52-support/)
- [How to use OSC 52 clipboard for copy only? – neovim discussion #28010](https://github.com/neovim/neovim/discussions/28010)
- [osc 52 – seankhliao](https://seankhliao.com/blog/12020-05-14-osc-52/)

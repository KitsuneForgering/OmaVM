# Cor do ambiente visível no Nautilus

## Situação

Ao mudar a cor de uma Machine, a OmaVM cria `~/OmaVM/<nome>` (link para o
disco) e grava a cor no xattr `user.xdg.tags`, como "best-effort"
(`CLAUDE.md`, Omarchy Integration). O gerenciador de arquivos do Omarchy é
o **Nautilus 50** (`SUPER + SHIFT + F` em
`/usr/share/omarchy/default/hypr/bindings/applications.lua`), e o Nautilus
não usa `user.xdg.tags`: ícones e marcas dele vêm dos **metadados do
GVFS**. Na prática a cor não aparece em lugar nenhum no Omarchy padrão.

## O que funciona

Conferido neste host (2026-09-28): `gio set -t string <link>
metadata::custom-icon file:///…/icone.svg` grava o metadado **no próprio
link** (não no disco apontado), e `gio info` o devolve. É o mesmo atributo
que o Nautilus grava quando o usuário escolhe um ícone personalizado.

## Esboço

- Um SVG por cor da paleta fixa (`core.EnvironmentColors`), gerado a partir
  do ícone da OmaVM, instalado junto com o app (sem gerar arquivos no
  `$HOME`).
- `Link` passa a gravar também `metadata::custom-icon` via `gio set`, com o
  mesmo tratamento best-effort do xattr: sem `gio` ou sem o daemon de
  metadados, a cor interna continua valendo e nada falha.
- `Unlink` não precisa limpar: os metadados do GVFS vão embora com o link.
- Manter o xattr para quem usa Dolphin/Baloo.

## Cuidados

- Há relatos de "Setting attribute metadata::custom-icon not supported" em
  alguns sistemas (depende do `gvfsd-metadata` e do sistema de arquivos);
  daí o best-effort.
- Desde 2026-09-28 o `Link` recusa sobrescrever um arquivo que não seja o
  link da própria OmaVM em `~/OmaVM`; o ícone só é gravado no link dela.

## Fontes

- [gio metadata::custom-icon – GNOME Discourse](https://discourse.gnome.org/t/gio-metadata-custom-icon-file-attribute-not-seen-by-shell-extension-code/7640)
- [Command-line hacking: assigning folder icons – Kevin Boone](https://kevinboone.me/clh_foldericon.html)
- [Folder Color: custom icons issue #35](https://github.com/costales/folder-color/issues/35)
- [GNOME Files – Wikipedia](https://en.wikipedia.org/wiki/GNOME_Files)

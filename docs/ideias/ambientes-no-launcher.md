# Ambientes no launcher do Omarchy

## Contexto

O Omarchy 4 "Quattro" (agosto de 2026) trouxe barra, launcher e menus para
um único processo Quickshell com plugins; Walker e Elephant saíram do
padrão. Os tipos de plugin aceitos são `bar-widget`, `panel`, `overlay`,
`menu`, `service` e `bar` (conferido em
`/usr/share/omarchy/shell/README.md` deste host). **Não existe** um tipo de
plugin para acrescentar resultados de busca ao launcher. Os plugins `menu`
recebem a biblioteca de aplicativos do host, ou seja, as entradas
`.desktop`.

## Ideia

Hoje, para abrir um ambiente é preciso abrir o Experience Center primeiro.
Uma entrada `.desktop` por ambiente ("Fedora — OmaVM") faz o ambiente
aparecer no launcher do Omarchy, e em qualquer outro, como um app comum. É
o mesmo mecanismo que o Distrobox usa (`distrobox generate-entry`) e que a
OmaVM já usa para exportar apps de Boxes (Blend Mode base).

## Esboço

- Opt-in por ambiente ("Mostrar no launcher", Settings → Automation),
  persistido no domínio como as outras preferências.
- `Exec=` aponta para o caminho que já existe: `omavm open NAME` para
  Machines (que lança o viewer) e `omavm-gui --terminal NAME` para Boxes (o
  `omavm open` de Box é interativo e precisa de TTY).
- Ícone por tipo e, quando houver, pela cor do ambiente (Color tags).
- `Remove` e renomear apagam ou atualizam a entrada; nunca deixar atalho
  órfão apontando para um ambiente que não existe.
- Um painel no plugin de barra (`contrib/dev.omavm.bar`) listando os
  ambientes com Open/Start é a alternativa sem arquivos no disco; exige o
  plugin habilitado, que é sempre manual (ver `CLAUDE.md`).

## Fontes

- [Arch-Based Omarchy 4.0 "Quattro" – Linuxiac](https://linuxiac.com/arch-based-omarchy-4-0-quattro-is-here-with-its-biggest-desktop-overhaul-yet/)
- [Omarchy Quattro gives Linux a desktop you can rearrange – Botmonster](https://botmonster.com/self-hosting/omarchy-quattro-release/)
- [Custom Walker Menus with Lua – Hans Schnedlitz](https://www.hansschnedlitz.com/writing/2026/02/22/custom-walker-menus-with-lua)

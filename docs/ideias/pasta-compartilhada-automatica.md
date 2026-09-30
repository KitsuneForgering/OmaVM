# Pasta compartilhada que aparece sozinha no guest

## Situação

A Machine compartilha uma pasta do host via virtiofs, mas o guest precisa
montar a tag `omavm-share` à mão (Settings: "Mount the tag omavm-share in
the guest"). É o tipo de detalhe de infraestrutura (tag, virtiofs, mount)
que o `CLAUDE.md` quer fora do fluxo básico (UX Principle #3).

## O que mudou no kernel

Desde a série de Stefan Hajnoczi (2024), o kernel expõe as tags em
`/sys/fs/virtiofs/<n>/tag` e emite um uevent quando um dispositivo virtiofs
aparece. Com isso, o userspace do guest consegue descobrir e montar as
pastas sem configuração prévia do nome da tag, e o systemd consegue
esperar a pasta durante o boot.

## Esboço

A OmaVM não roda nada dentro do guest (`omavm-guest` continua fora do
escopo), então não pode montar sozinha. O que cabe:
- Uma **regra udev + unidade systemd** pequena e documentada que o usuário
  instala no guest uma vez, montando qualquer tag `omavm-*` em
  `~/OmaVM/<tag>` ao ver o uevent. Entregue como arquivo em `contrib/`,
  opt-in, como a regra do Hyprland.
- Nas Settings, trocar o texto sobre a tag por "Para ver a pasta dentro do
  sistema, instale o ajudante da OmaVM no guest" com link para as
  instruções, e mostrar se o guest já montou a pasta quando o QGA estiver
  conectado (o QGA pode listar montagens sem agente próprio).
- Depende de kernel 6.9 ou mais novo no **guest**; em guests antigos, o
  texto atual (montar à mão) continua valendo.

## Relacionado

Desde 2026-09-28, uma pasta que sumiu do host é avisada no Start com o
caminho e onde corrigir, e o erro do `virtiofsd` fica em
`<estado>/machines/<nome>/virtiofsd.log`.

## Fontes

- [virtiofs: export filesystem tags through sysfs – LWN](https://lwn.net/Articles/961448/)
- [virtiofs — The Linux Kernel documentation](https://docs.kernel.org/filesystems/virtiofs.html)
- [virtiofs standalone usage (QEMU howto)](https://virtio-fs.gitlab.io/howto-qemu.html)

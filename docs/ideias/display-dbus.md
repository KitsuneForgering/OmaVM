# Próximos passos do display D-Bus

O viewer migrou para o D-Bus display do QEMU em 2026-09-28 (ver
`CLAUDE.md`). Ideias levantadas na migração e na pesquisa:

## Preview sem `screendump`

O `screendump` via QMP falha com "no surface" em alguns guests (Haiku) com
display GL. O viewer já recebe os quadros; o preview do Experience Center
poderia vir de uma conexão D-Bus curta (um `ScanoutDMABUF`/`ScanoutMap`
basta) em vez do `screendump`. Resolve o preview que falta hoje no Haiku.

Evidência (2026-09-28): numa Machine Haiku iniciada pelo `Start` novo
(`dbus,p2p=yes,gl=on`), o `screendump` falhou com "no surface" durante e
depois do boot, igual ao que acontecia com `egl-headless`; o display em si
funcionava (o console reportava 1280x800). Com o GL ligado, o scanout do
guest é uma textura, sem a superfície 2D que o `screendump` lê. O custo da
alternativa: no modo GL o quadro é um dma-buf (tiled na Intel), que só a GPU
lê; a miniatura teria de ser gerada do lado Qt (onde a importação EGL já
existe), não no Go.

## Atualizações só da área alterada (damage)

O viewer redesenha a textura inteira a cada `UpdateDMABUF`. O trabalho
recente de "damage areas" no espaço VirtIO propaga só as regiões alteradas
do guest até o cliente, reduzindo o custo por quadro. Com dma-buf a textura
já é compartilhada, mas repassar o dano ao scene graph evita recompor a
janela inteira.

## Redirecionamento USB e clipboard com imagens

O Boxes ainda depende do SPICE porque o libmks (cliente D-Bus) não tem
redirecionamento USB. O `ui/dbus-display1.xml` também só define clipboard
de texto (`text/plain;charset=utf-8`). Nenhum dos dois é possível hoje só
com o D-Bus display: registrar como limite, sem prometer.

## Áudio pelo próprio display

Sem `-audiodev`, o QEMU usa o áudio D-Bus por padrão com `-display dbus`.
A OmaVM usa PipeWire direto no host, que já funciona sem o viewer aberto.
Manter assim; anotado só para não "corrigir" por engano.

## Fontes

- [D-Bus display — QEMU documentation](https://www.qemu.org/docs/master/interop/dbus-display)
- [Damage areas across the VirtIO space – Bilal Elmoussaoui](https://belmoussaoui.com/blog/16-damage-areas-across-the-virtio-space/)
- [qemu-display (rdw, libmks)](https://github.com/rustdesk-org/qemu-display)

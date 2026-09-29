# Windows 11 sem ajustes manuais (TPM + Secure Boot)

## O que outros fazem

O novo GNOME Boxes instala o Windows 11 sem workarounds: configura Secure
Boot (firmware UEFI/OVMF com chaves) e um TPM virtual automaticamente. O
Quickemu também cria VMs Windows e macOS "out of the box".

## Encaixe no escopo

A OmaVM "não é uma ferramenta exclusiva para Windows" (Scope/Non-Goals),
mas Windows é um caso real de Machine. O instalador do Windows 11 recusa
máquinas sem TPM 2.0 e Secure Boot. Hoje a Machine usa BIOS (SeaBIOS) e não
tem TPM, então essa instalação falha, e o usuário não tem como descobrir o
motivo pela UI.

## Esboço

- Firmware UEFI (OVMF) como padrão das Machines novas, com `OVMF_VARS`
  próprio por Machine no diretório de estado.
- `swtpm` por Machine (processo auxiliar, como o `virtiofsd` já é), ligado
  por `-tpmdev emulator`.
- `omavm host` reporta se OVMF e `swtpm` estão instalados.
- A escolha entre BIOS e UEFI é detalhe interno, não uma pergunta na criação
  (UX Principle #4); Machines existentes continuam como estão.

## Riscos

- Trocar BIOS por UEFI numa Machine já instalada quebra o boot: só vale para
  Machines novas.
- O `swtpm` guarda estado sensível (chaves do BitLocker, por exemplo): o
  diretório precisa de permissões restritas e entrar no Remove e no Clone.

## Fontes

- [The Future of GNOME Boxes – Felipe Borges](https://blogs.gnome.org/feborges/future-of-boxes/)
- [Quickemu – Chris Titus Tech](https://christitus.com/quickemu/)

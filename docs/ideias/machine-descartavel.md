# Machine descartável com `-snapshot` do QEMU

## O que outros fazem

O UTM 4 tem um modo de VM descartável: a VM roda normalmente, e nada do
que acontece nela sobrevive ao desligamento.

## Encaixe na OmaVM

O `CLAUDE.md` trata ambientes descartáveis como conceito de primeira classe
(`omavm run arch --ephemeral`, seção Ephemeral Environments & Agentic
Workflows), para Boxes e Machines "quando possível". Para Machines o QEMU já
resolve isso: com `-snapshot`, as escritas no disco vão para um overlay
temporário descartado quando a VM termina. É integração de um recurso
maduro, não um mecanismo novo (Backend Rules).

## Esboço

- `omavm start NAME --ephemeral` (e depois `omavm run ... --ephemeral`)
  passa `-snapshot` no Start daquela sessão, sem persistir nada no
  domínio.
- O card mostra que a sessão é descartável ("As alterações desta sessão
  serão perdidas ao desligar"), sem esconder a consequência.
- Casos de uso: testar um instalador, reproduzir um bug em sistema limpo,
  deixar um agente mexer numa Machine sem risco para o disco.

## Riscos

- `savevm` (Snapshot Manager) durante uma sessão `-snapshot` grava no
  overlay temporário: o snapshot sumiria ao desligar. O Snapshot Manager
  precisa recusar ou avisar nessa sessão.
- O overlay fica em `/var/tmp` por padrão: sessões longas podem encher o
  disco do host (ver [disco-do-host.md](disco-do-host.md)).

## Fontes

- [UTM 4.0 release notes](https://docs.getutm.app/updates/v4.0/)
- [UTM (software) – Wikipedia](https://en.wikipedia.org/wiki/UTM_(software))

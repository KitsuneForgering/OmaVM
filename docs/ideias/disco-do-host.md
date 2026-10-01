# Espaço em disco real do host

## O que outros fazem

No Parallels Desktop 26, a VM Windows passa a ver o espaço realmente livre
no Mac, para evitar travamentos quando o disco do host enche durante
instalações grandes.

## O problema na OmaVM

A Machine usa um qcow2 esparso de tamanho fixo (`defaultDiskSize`). O guest
vê o disco inteiro como livre mesmo quando o disco do host está quase cheio;
quando o qcow2 não consegue crescer, o QEMU pausa a VM com `io-error`, e o
card mostra só "Error".

## Esboço

**Aviso de pouco espaço implementado (2026-10-01)**: abaixo de 4 GiB livres, o card e `omavm status` avisam que a Machine pausa se o espaço acabar, antes do Start e com ela rodando. Não compara com o crescimento possível do qcow2: com 1 TiB esparso, isso avisaria sempre.

**Primeiro item implementado (2026-09-29)**, junto com o disco padrão de
1 TiB esparso: um `io-error` aparece como pausa com o motivo ("o disco
deste computador está cheio; libere espaço e retome"), e Resume continua a
Machine. O aviso antes do Start e o redimensionar/compactar abaixo seguem
como ideia.

- Ao pausar por `io-error` (o `statusFromQMP` já distingue esse estado), o
  Core checa o espaço livre do sistema de arquivos do diretório de estado e
  explica: "o disco do computador encheu; libere espaço e retome".
- Aviso antes do Start quando o espaço livre do host for menor que o
  crescimento possível do qcow2 (tamanho virtual menos o alocado).
- Não repetir o que o Parallels faz dentro do guest: exigiria um agente
  (`omavm-guest`), que continua fora do escopo.

## Redimensionar e compactar o disco

O UTM 4 permite, nas configurações do disco, **redimensionar** o qcow2 e
**compactá-lo** (reescrevendo só os blocos em uso). Na OmaVM, os dois são
comandos maduros do `qemu-img` (`resize`, `convert -c`) com a Machine
parada:
- aumentar o disco quando o guest encher, em vez de recriar a Machine
  (o guest ainda precisa expandir a própria partição; a UI deve dizer isso);
- devolver espaço ao host depois de apagar arquivos no guest, o que
  combina com o aviso de disco cheio acima.

O custo aparece na UI (UX Principle #10): compactar reescreve o arquivo
inteiro e precisa de espaço livre equivalente enquanto roda.

## Fontes

- [UTM 4.0 release notes](https://docs.getutm.app/updates/v4.0/)

- [Parallels Desktop 26 updates summary – Parallels KB](https://kb.parallels.com/en/131014)
- [Parallels Desktop 26 – 9to5Mac](https://9to5mac.com/2025/08/26/parallels-desktop-26-brings-macos-26-support-and-new-tools-for-it/)

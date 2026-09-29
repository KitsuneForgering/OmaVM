# Arquivar e exportar ambientes

## O que outros fazem

- **Parallels Desktop — Archive:** uma VM pouco usada é compactada para
  ocupar bem menos disco; enquanto arquivada, não inicia, e "Unarchive" a
  devolve.
- **UTM:** a VM inteira (disco + configuração) vira um único `.utm`,
  compartilhado dentro de um `.zip`, que outro Mac importa.

## Encaixe na OmaVM

Hoje uma Machine são arquivos soltos no diretório de estado (qcow2, pid,
sockets) mais uma entrada no `environments.json`. Não há como:
- liberar espaço de uma Machine parada sem apagá-la;
- levar uma Machine para outro computador ou guardá-la fora do host, além
  de copiar o qcow2 à mão e recriar a configuração.

## Esboço

- **Arquivar (Machine parada):** `qemu-img convert -c` para um qcow2
  compactado no mesmo diretório e marca no domínio (`Archived`) que
  bloqueia o `Start` com uma mensagem clara e oferece "Restaurar". Mesmo
  mecanismo da ideia de compactar disco ([disco-do-host.md](disco-do-host.md)),
  com estado explícito.
- **Exportar/Importar:** `omavm export NOME arquivo.omavm` gera um tar com o
  qcow2 (snapshots internos incluídos) e um manifesto com a configuração
  do ambiente, **sem** caminhos do host (ISO, pasta compartilhada) nem
  estado de runtime. `omavm import` recria o ambiente com nome e ID novos.
  O formato do manifesto esbarra em "Template format" das Open Technical
  Decisions do `CLAUDE.md`: decidir antes, não de carona.
- **Boxes:** o equivalente é `podman commit` + `podman save`; o `$HOME`
  compartilhado não viaja junto, e isso precisa aparecer na UI.

## Relacionado

Desde 2026-09-28 o registro (`environments.json`) é gravado com `fsync` e
mantém a versão anterior em `environments.json.bak`, para recuperação após
queda de energia.

## Fontes

- [Parallels Desktop Help – Archive and Unarchive Virtual Machines](https://download.parallels.com/desktop/v19/docs/en_US/Parallels%20Desktop%20User's%20Guide/42000.htm)
- [UTM on Mac: When to Consider Parallels Desktop](https://www.parallels.com/compare/utm/)
- [UTM_to_Parallels_importer](https://github.com/MRX7999/UTM_to_Parralels_importer)

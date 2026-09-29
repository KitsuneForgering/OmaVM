# Lista de ambientes como model C++

## Situação

Em 2026-09-28 o Experience Center parou de recriar todos os cards a cada
mudança de status: `gui/EnvironmentListModel.qml` sincroniza um `ListModel`
no lugar, por nome, a partir do `backend.environments` (array). A
documentação do Qt confirma o motivo do bug: com um array JS como model, o
`ListView` reseta e cria delegates novos para tudo; `ListModel` e
`QAbstractListModel` atualizam no lugar.

## Próximo passo possível

O jeito idiomático do Qt é o `Backend` expor um `QAbstractListModel`:
- o C++ já sabe exatamente qual ambiente e qual campo mudou
  (`enrichEnvironment` atualiza status, guest tools e preview por índice);
  hoje ele emite `environmentsChanged` para a lista inteira e o QML
  reconstrói a diferença comparando nomes;
- com um model C++, cada mudança vira um `dataChanged` só daquela linha e
  daquele papel (`status`, `preview`…), sem cópia da lista nem comparação
  em JS;
- os diálogos (Snapshots, Settings) que hoje procuram o ambiente por nome
  em `backend.environments` passariam a usar o mesmo model.

## Custo e cuidado

- Mais código C++ (papéis, `rowCount`, `data`, `beginInsertRows`…) e os
  testes de `gui/tests/backend_refresh_test.cpp` passam a olhar sinais do
  model em vez de `environmentsChanged`.
- Há relatos de `dataChanged` não repintar delegates com os quais o usuário
  interagiu; conferir com o card real (hover, menu aberto) antes de trocar.
- Só vale a pena se a sincronização em QML ficar lenta ou complicada; com
  até ~30 ambientes (limite de desenho do Experience Center) ela é barata.

## Fontes

- [Models and Views in Qt Quick – Qt 6.11](https://doc.qt.io/qt-6/qtquick-modelviewsdata-modelview.html)
- [ListView dataChanged not updating all delegates – Qt Forum](https://forum.qt.io/topic/67523/listview-datachanged-not-updating-all-delegates)
- [Model-View-Delegate – QML Book](https://qmlbook.github.io/ch07-modelview/modelview.html)

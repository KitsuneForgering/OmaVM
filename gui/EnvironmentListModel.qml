import QtQuick

// The Experience Center's list, kept in sync with backend.environments in
// place. Using the JS array as the ListView model reset the whole list on
// every change (a status, a preview arriving): all cards were destroyed
// and faded in again, and an open card menu disappeared mid-use. Here each
// environment keeps its row (matched by name, its stable identity) and
// only its data is replaced, so its card lives on.
ListModel {
    id: model
    dynamicRoles: true

    function indexOf(name) {
        for (let i = 0; i < count; ++i)
            if (get(i).environment.name === name)
                return i
        return -1
    }

    function sync(environments) {
        const names = environments.map(e => e.name)
        for (let i = count - 1; i >= 0; --i)
            if (names.indexOf(get(i).environment.name) === -1)
                remove(i)
        for (let i = 0; i < environments.length; ++i) {
            const at = indexOf(environments[i].name)
            if (at === -1) {
                insert(i, { environment: environments[i] })
                continue
            }
            if (at !== i)
                move(at, i, 1)
            setProperty(i, "environment", environments[i])
        }
    }
}

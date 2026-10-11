/** Workspace (board catalog) y sincronización de tablero compartido. */
export default {
  catalog: {
    loadFailed: 'No se pudo cargar la lista de lienzos. Inténtelo de nuevo.',
    reloadRequired: 'La lista de lienzos necesita recargarse',
    reloading: 'Recargando…',
    reload: 'Recargar',
  },
  deleteDialog: {
    title: 'Eliminar lienzo',
    description: '¿Eliminar "{name}"? Esta acción no se puede deshacer.',
    deleting: 'Eliminando…',
  },
  feedback: {
    createSyncConflict: 'Se produjo un conflicto de sincronización. Inténtelo de nuevo.',
    createSyncFailed: 'No se pudo crear el lienzo. Inténtelo de nuevo más tarde.',
    nameAlreadyExists: 'El nombre ya existe',
    nameEmpty: 'El nombre del lienzo no puede estar vacío',
    renameSyncConflictReloaded: 'Conflicto de sincronización. Se ha recargado la lista de lienzos.',
    renameSyncFailedRolledBack:
      'Error al sincronizar el cambio de nombre. Se restauró el nombre original.',
    renameSyncConflictRecoveryIncomplete:
      'Conflicto de sincronización y recuperación incompleta. Recargue la lista de lienzos.',
    renameSyncFailedNoRollback:
      'Error al sincronizar el cambio de nombre y no se pudo restaurar. Recargue la lista de lienzos.',
    renamed: 'Lienzo renombrado',
    copyFailed: 'Error al copiar',
    duplicateSyncConflict:
      'Se produjo un conflicto de sincronización al copiar. Inténtelo de nuevo.',
    duplicateSyncFailed: 'Error al sincronizar la copia. Inténtelo de nuevo más tarde.',
    duplicated: 'Lienzo copiado',
    deleteSyncFailed: 'Error al eliminar. Inténtelo de nuevo más tarde.',
    deleteFailed: 'Error al eliminar',
    deleted: 'Lienzo eliminado',
    syncFailedRetry: 'Error al sincronizar el lienzo. Inténtelo de nuevo más tarde.',
    reloaded: 'Lista de lienzos recargada',
    noWorkspaceSelected:
      'No hay ningún espacio de trabajo seleccionado. Guardado localmente; seleccione un espacio de trabajo e inténtelo de nuevo.',
  },
  joinSharedBoard: {
    missingToken: 'Falta el token de recurso compartido',
    joinFailed: 'No se pudo unir al lienzo compartido',
    missingBoardId: 'El servidor no devolvió un ID de lienzo válido',
    hydrateFailed: 'Error al sincronizar el lienzo. Inténtelo de nuevo más tarde.',
  },
  sync: {
    listFailed: 'No se pudo sincronizar la lista de lienzos',
    saveFailed: 'No se pudo guardar el lienzo',
    deleteFailed: 'No se pudo eliminar el lienzo',
  },
  // Alias TTL: `workspace.boards.*` duplicates the `board.*` namespace until
  // the end of P4; deletion owner: P4 board-lane. New callers must use `board.*`.
  boards: {
    title: '¡Te damos la bienvenida a IdeaBoard!',
    subtitle: 'Explora, amplía y perfecciona tus ideas',
    newCanvas: 'Nuevo lienzo',
    searchPlaceholder: 'Buscar lienzos...',
    searchLabel: 'Buscar lienzos',
    sortLabel: 'Ordenar lienzos',
    sortRecent: 'Reciente',
    sortName: 'Nombre',
    createNewCanvas: 'Crear nuevo lienzo',
    startFromEmpty: 'Empieza desde un lienzo vacío',
    noMatchingCanvas: 'No hay lienzos coincidentes',
    untitledCanvas: 'Lienzo sin título',
    moreActions: 'Más acciones',
    duplicate: 'Duplicar',
    edited: 'Editado {date}',
  },
} as const

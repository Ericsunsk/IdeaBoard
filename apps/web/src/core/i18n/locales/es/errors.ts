/**
 * Copias en español indexadas por códigos de error estables del backend.
 */
export default {
  submission_invalid: 'Revise la solicitud e inténtelo de nuevo.',
  submission_failed: 'No se pudo enviar la solicitud. Inténtelo de nuevo.',
  insufficient_credits: 'Necesita más Ideas para ejecutar esta acción.',
  pricing_unavailable: 'El precio no está disponible temporalmente. Inténtelo de nuevo en breve.',
  provider_unavailable:
    'Este servicio no está disponible en este momento. Inténtelo de nuevo más tarde.',
  service_resolution_unavailable: 'Esta resolución no está disponible en este momento.',
  prompt_unavailable:
    'Esta indicación no está disponible en este momento. Inténtelo de nuevo más tarde.',
  wallet_restricted: 'Las Ideas no están disponibles temporalmente para esta cuenta.',
  member_quota_exhausted: 'Has agotado tu cuota personal de este período.',
  vision_search_unavailable:
    'La Búsqueda Visual no está disponible en este momento. Inténtelo de nuevo más tarde.',
  vision_search_input_invalid: 'Revise su búsqueda e inténtelo de nuevo.',
  vision_search_provider_error: 'Error en la Búsqueda Visual. Inténtelo de nuevo.',
  idempotency_key_required: 'No se pudo iniciar la solicitud. Inténtelo de nuevo.',
  idempotency_conflict: 'Esta solicitud ya está en curso.',
  checkout_request_rejected: 'No se pudo iniciar el pago. Inténtelo de nuevo.',
  checkout_session_expired: 'Esta sesión de pago ha expirado. Inicie un nuevo pago.',
  payment_already_completed: 'Este pago ya se ha completado.',
  board_forbidden: 'No puedes operar este tablero.',
  task_forbidden: 'No puedes operar esta tarea.',
  board_archived: 'Este tablero está archivado y no acepta esta operación.',
  board_not_found: 'Tablero no encontrado.',
  board_access_conflict: 'El acceso cambió en otro lugar. Actualiza e inténtalo de nuevo.',
  board_precondition_required:
    'El estado de acceso está desactualizado. Actualiza e inténtalo de nuevo.',
  board_member_not_found: 'Miembro del tablero no encontrado.',
  organization_not_found: 'Organización no encontrada.',
  organization_member_not_found: 'Miembro de la organización no encontrado.',
  organization_limit_reached: 'Cada usuario puede unirse o crear como máximo una organización.',
  organization_closed: 'Esta organización está cerrada.',
  organization_closure_blocked:
    'Aún hay tareas o fondos sin liquidar. Liquídalos e inténtalo de nuevo.',
  membership_transition_forbidden: 'El último gestor no se puede quitar ni degradar.',
} as const

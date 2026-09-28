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
  vision_search_not_configured: 'La Búsqueda Visual no está disponible en este momento.',
  vision_search_unavailable:
    'La Búsqueda Visual no está disponible en este momento. Inténtelo de nuevo más tarde.',
  vision_search_input_invalid: 'Revise su búsqueda e inténtelo de nuevo.',
  vision_search_image_invalid: 'Esta imagen no se puede usar para la Búsqueda Visual.',
  vision_search_image_unavailable: 'Esta imagen no está disponible. Pruebe con otra imagen.',
  vision_search_timeout: 'La Búsqueda Visual tardó demasiado. Inténtelo de nuevo.',
  vision_search_credentials: 'La Búsqueda Visual no está disponible en este momento.',
  vision_search_quota:
    'La Búsqueda Visual no está disponible temporalmente. Inténtelo de nuevo más tarde.',
  vision_search_network: 'No se pudo conectar con la Búsqueda Visual. Inténtelo de nuevo.',
  vision_search_provider_error: 'Error en la Búsqueda Visual. Inténtelo de nuevo.',
  vision_search_schema_error:
    'La Búsqueda Visual devolvió un resultado no válido. Inténtelo de nuevo.',
  idempotency_key_required: 'No se pudo iniciar la solicitud. Inténtelo de nuevo.',
  idempotency_conflict: 'Esta solicitud ya está en curso.',
  checkout_request_rejected: 'No se pudo iniciar el pago. Inténtelo de nuevo.',
  checkout_session_expired: 'Esta sesión de pago ha expirado. Inicie un nuevo pago.',
  payment_already_completed: 'Este pago ya se ha completado.',
} as const

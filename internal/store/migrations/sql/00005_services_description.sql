-- +goose Up
-- Descripción libre del servicio: contexto extra que se inyecta en el system
-- prompt para que el modelo pueda explicar/defender cada servicio.
ALTER TABLE services ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

-- Backfill de datos de prueba (precio, duración y descripción) para las filas ya
-- existentes. Solo rellena huecos: no sobrescribe precio/duración ya definidos.
-- NOTA: precios, duraciones y descripciones son placeholders de desarrollo;
-- deben validarse antes de publicar el catálogo en producción.
UPDATE services SET
    description  = v.description,
    price        = COALESCE(services.price, v.price),
    duration_min = COALESCE(services.duration_min, v.duration_min),
    updated_at   = now()
FROM (VALUES
    ('nano_tf_cejas',   'Micropigmentación de cejas pelo a pelo con efecto natural realizada por Tamara Freitas. Incluye diseño previo, anestesia tópica y revisión de retoque a las 4-6 semanas.', NULL::numeric, 120),
    ('nano_tf_ojos',    'Eyeliner permanente (superior o inferior) realizado por Tamara Freitas. Incluye diseño, anestesia tópica y revisión posterior.', NULL, 120),
    ('nano_tf_labios',  'Micropigmentación de labios para definir contorno y aportar color. Incluye diseño, anestesia tópica y revisión posterior.', NULL, 120),
    ('nano_eq_cejas',   'Micropigmentación de cejas pelo a pelo realizada por el equipo de la clínica. Incluye diseño, anestesia tópica y revisión de retoque.', NULL, 120),
    ('nano_eq_ojos',    'Eyeliner permanente realizado por el equipo. Incluye diseño, anestesia tópica y revisión.', NULL, 120),
    ('nano_eq_labios',  'Micropigmentación de labios (contorno y color) realizada por el equipo. Incluye diseño, anestesia tópica y revisión.', NULL, 120),
    ('nano_rep_cons',   'Sesión de repaso de micropigmentación solicitada fuera del plazo habitual. Requiere valoración previa del trabajo existente.', 50, 30),
    ('nano_rep_res',    'Sesión de repaso de micropigmentación con reserva previa obligatoria. Sujeta a valoración del trabajo existente.', 60, 45),
    ('nano_prueba',     'Prueba alérgica opcional de pigmentos antes de la micropigmentación. Debe realizarse al menos 48 horas antes de la sesión.', NULL, 15),
    ('ext_lab_anes',    'Refuerzo de anestesia médica tópica para sesiones de micropigmentación de labios más largas o sensibles.', NULL, 30),
    ('micro_cap_cons',  'Micropigmentación capilar para disimular calvicie o aportar densidad visual. Requiere valoración previa de la zona a tratar.', 300, 180),
    ('rem_las_cejas',   'Eliminación con láser de micropigmentación antigua en cejas (protocolo DETOX SOS). Incluye valoración de la zona.', NULL, 30),
    ('rem_der_detox',   'Eliminación de micropigmentación con dermapen DETOX SOS. Alternativa no láser para pigmentos resistentes.', NULL, 45),
    ('rem_las_eye',     'Eliminación con láser de eyeliner permanente antiguo. Se realiza con protección ocular.', NULL, 30),
    ('rem_las_tatoo',   'Eliminación de tatuajes con láser. Requiere valoración previa y puede necesitar varias sesiones.', 90, 45),
    ('fac_hig_90',      'Higiene facial profunda: limpieza, exfoliación, extracción de comedones, mascarilla y protección final.', NULL, 90),
    ('fac_rev_90',      'Revitalización facial con activos hidratantes y antioxidantes para devolver luminosidad y firmeza.', NULL, 90),
    ('fac_pel_45',      'Peeling facial químico para renovar la piel, mejorar la textura y unificar el tono.', NULL, 45),
    ('fac_derm_60',     'Dermapen facial con microneedling para estimular el colágeno y mejorar poros y cicatrices. Puede requerir varias sesiones.', NULL, 60),
    ('fac_dre_30',      'Drenaje facial manual para reducir la retención de líquidos y mejorar el contorno del rostro.', NULL, 30),
    ('fac_crio_full',   'Protocolo completo de crioestimulación facial para tensar, oxigenar y aportar luminosidad. Sesión extensa con varias fases.', NULL, 240),
    ('fac_rev_0',       'Revisión estética facial gratuita para valorar la piel y definir el tratamiento adecuado.', NULL, 30),
    ('corp_dre_bra',    'Masaje Sliim Redux de drenaje brasileño para reducir la retención de líquidos y mejorar el contorno corporal.', NULL, 60),
    ('corp_cel_cero',   'Protocolo intensivo anticelulitis con aparatología y masaje. Recomendado en ciclos de varias sesiones.', NULL, 180),
    ('corp_detox_90',   'Tratamiento detox corporal para eliminar toxinas y activar la circulación. Incluye masaje y envoltura.', NULL, 90),
    ('corp_relax_60',   'Masaje relajante de cuerpo completo para aliviar la tensión muscular y el estrés.', NULL, 60),
    ('corp_liped_180',  'Tratamiento corporal específico para lipedema con drenaje linfático manual adaptado.', NULL, 180),
    ('corp_cav_60',     'Cavitación por ultrasonidos para reducir grasa localizada y mejorar la piel. Recomendada en ciclos.', NULL, 60),
    ('corp_crio_din',   'Criodinámica (frío controlado) para reducir grasa localizada y activar la circulación.', NULL, 60),
    ('corp_crio_esc',   'Crioescultura corporal con frío localizado para modelar zonas rebeldes. Requiere diagnóstico previo y varias sesiones.', 120, 60),
    ('las_w_bas',       'Pack básico de depilación láser diodo para mujer: axilas, ingles, pubis, labios y perianal.', NULL, 30),
    ('las_w_med',       'Pack medio de láser diodo para mujer: básico más media pierna y pies.', NULL, 45),
    ('las_w_com',       'Pack completo de láser diodo para mujer: básico más pierna completa y pies.', NULL, 60),
    ('las_w_full',      'Depilación láser diodo de cuerpo completo para mujer. Incluye todas las zonas del pack completo.', NULL, 90),
    ('las_m_bas',       'Pack básico de láser diodo para hombre: pecho, abdomen y hombros.', NULL, 30),
    ('las_m_med',       'Pack medio de láser diodo para hombre: básico más espalda, nuca, brazos, axilas y manos.', NULL, 60),
    ('las_m_com',       'Pack completo de láser diodo para hombre: piernas, pies, axilas, pecho, abdomen, hombros, brazos y manos.', NULL, 75),
    ('las_m_full',      'Depilación láser diodo de cuerpo completo para hombre. Incluye todas las zonas del pack completo.', NULL, 90),
    ('las_z_mini',      'Sesión de láser diodo en una zona mini (por ejemplo labio superior o dedos). Precio por zona.', NULL, 15),
    ('las_z_peq',       'Sesión de láser diodo en una zona pequeña (por ejemplo axilas o ingles). Precio por zona.', NULL, 20),
    ('las_z_med',       'Sesión de láser diodo en una zona media (por ejemplo media pierna o antebrazo). Precio por zona.', NULL, 30),
    ('las_z_comb',      'Sesión de láser diodo en varias zonas combinadas. Precio por conjunto de zonas.', NULL, 45),
    ('las_z_suel',      'Sesión de láser diodo en una zona suelta (por ejemplo pierna completa o espalda). Precio por zona.', NULL, 45),
    ('las_z_gran',      'Sesión de láser diodo en una zona grande (por ejemplo espalda completa y abdomen). Precio por zona.', NULL, 60),
    ('res_cita_micro',  'Reserva de cita de micropigmentación; el importe se descuenta del precio del servicio.', NULL, 30),
    ('eval_prim_vis',   'Primera visita de evaluación gratuita para nuevos clientes de micropigmentación. Se valora el trabajo previo y se resuelven dudas.', NULL, 30),
    ('eval_vis_est',    'Primera visita estética corporal o facial gratuita. Diagnóstico y propuesta de tratamiento.', NULL, 30),
    ('mod_formacion',   'Sesión como modelo para prácticas de formación del equipo. Precio reducido y condiciones específicas.', NULL, 60)
) AS v (id, description, price, duration_min)
WHERE services.id = v.id;

-- Las notas que decían "a consultar" dejan de contradecir el precio de prueba.
UPDATE services SET note = 'Precio orientativo sujeto a confirmación', updated_at = now()
WHERE id IN ('nano_rep_cons', 'micro_cap_cons', 'rem_las_tatoo', 'corp_crio_esc');
UPDATE services SET note = 'Requiere reserva previa', updated_at = now()
WHERE id = 'nano_rep_res';

-- +goose Down
ALTER TABLE services DROP COLUMN IF EXISTS description;

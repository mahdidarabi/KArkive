package resources

import (
	karkivev1alpha1 "github.com/mahdidarabi/Karkive/api/v1alpha1"
)

// EffectiveEngine treats an empty value as postgres (CRD default).
func EffectiveEngine(engine karkivev1alpha1.Engine) karkivev1alpha1.Engine {
	if engine == "" {
		return karkivev1alpha1.EnginePostgres
	}
	return engine
}

// EngineImplemented reports whether the operator can reconcile this engine.
// Phase 1: postgres backup only.
func EngineImplemented(engine karkivev1alpha1.Engine) bool {
	return EffectiveEngine(engine) == karkivev1alpha1.EnginePostgres
}

// DumpPrefix is the object-name prefix used by pipeline scripts.
func DumpPrefix(engine karkivev1alpha1.Engine) string {
	switch EffectiveEngine(engine) {
	case karkivev1alpha1.EngineMariaDB:
		return "mysqldump"
	case karkivev1alpha1.EngineRedis:
		return "redisdump"
	default:
		return "pg_dump"
	}
}

// DefaultPort for the engine when spec.database.port is unset.
func DefaultPort(engine karkivev1alpha1.Engine) int32 {
	switch EffectiveEngine(engine) {
	case karkivev1alpha1.EngineMariaDB:
		return 3306
	case karkivev1alpha1.EngineRedis:
		return 6379
	default:
		return 5432
	}
}

func DatabasePort(db karkivev1alpha1.DatabaseSpec, engine karkivev1alpha1.Engine) int32 {
	if db.Port > 0 {
		return db.Port
	}
	return DefaultPort(engine)
}

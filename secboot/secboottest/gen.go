package secboottest

//go:generate mockgen -package secboottest -destination backend.go .. SecbootBackend
//go:generate mockgen -package secboottest -destination key_loading_backend.go .. SecbootKeyLoadingBackend
//go:generate mockgen -package secboottest -destination unlocking_backend.go .. SecbootUnlockingBackend
//go:generate mockgen -package secboottest -destination hooks_keydata.go .. SecbootHooksKeyDataSetter
//go:generate mockgen -package secboottest -destination seled_keydata.go .. SecbootSealedKeyDataActor
//go:generate mockgen -package secboottest -destination seled_keyobject.go .. SecbootSealedKeyObjectActor
//go:generate mockgen -package secboottest -destination keydata.go .. SecbootKeyDataActor

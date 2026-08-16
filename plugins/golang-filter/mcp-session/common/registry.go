package common

import "reflect"

var GlobalRegistry = NewServerRegistry()

type Server interface {
	ParseConfig(config map[string]any) error
	NewServer(serverName string) (*MCPServer, error)
}

// ServerCloner returns an independent config instance for parsing one server entry.
// Implement this when a server config contains maps, slices, pointers, or other
// mutable nested state that must not be shared between parse attempts.
type ServerCloner interface {
	Clone() Server
}

type ServerRegistry struct {
	servers map[string]Server
}

func NewServerRegistry() *ServerRegistry {
	return &ServerRegistry{
		servers: make(map[string]Server),
	}
}

func (r *ServerRegistry) RegisterServer(name string, server Server) {
	r.servers[name] = server
}

func (r *ServerRegistry) UnregisterServer(name string) {
	delete(r.servers, name)
}

func (r *ServerRegistry) GetServer(name string) Server {
	return r.servers[name]
}

func (r *ServerRegistry) NewServerConfig(name string) Server {
	server := r.GetServer(name)
	if server == nil {
		return nil
	}
	if cloner, ok := server.(ServerCloner); ok {
		return cloner.Clone()
	}

	value := reflect.ValueOf(server)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return nil
	}

	copy := reflect.New(value.Elem().Type())
	copy.Elem().Set(value.Elem())
	cloned, ok := copy.Interface().(Server)
	if !ok {
		return nil
	}
	return cloned
}

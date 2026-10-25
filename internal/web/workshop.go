package web

import (
	"net/http"
	"strings"
)

// CS2 Workshop maps of a server (Maps tab).

func (s *Server) workshopAdd(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Input string `json:"input"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	res, err := s.M.AddWorkshop(r.PathValue("id"), strings.TrimSpace(in.Input), u.Name)
	if err != nil {
		return fail(http.StatusBadRequest, "%s.", strings.TrimSuffix(capitalize(err.Error()), "."))
	}
	return writeJSON(w, res)
}

func (s *Server) workshopRemove(w http.ResponseWriter, r *http.Request, u *User) error {
	if err := s.M.RemoveWorkshop(r.PathValue("id"), r.PathValue("wid"), u.Name); err != nil {
		return fail(http.StatusBadRequest, "%s.", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) workshopStart(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if err := s.M.SetWorkshopStart(r.PathValue("id"), strings.TrimSpace(in.ID), u.Name); err != nil {
		return fail(http.StatusBadRequest, "%s.", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) workshopCollection(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Input string `json:"input"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	title, err := s.M.SetWorkshopCollection(r.PathValue("id"), strings.TrimSpace(in.Input), u.Name)
	if err != nil {
		return fail(http.StatusBadRequest, "%s.", strings.TrimSuffix(capitalize(err.Error()), "."))
	}
	return writeJSON(w, map[string]any{"ok": true, "title": title})
}

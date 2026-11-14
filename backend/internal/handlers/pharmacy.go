package handlers

import (
	"net/http"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

// registerPharmacy mounts the pharmacy B2C routes.
func (h *Handlers) registerPharmacy(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/branches", h.listBranches)
	mux.HandleFunc("GET /api/v1/branches/{id}", h.getBranch)
	mux.HandleFunc("GET /api/v1/branches/{id}/inventory", h.listBranchInventory)
	mux.HandleFunc("GET /api/v1/medications", h.searchMedications)
	mux.HandleFunc("GET /api/v1/medications/{id}", h.getMedicationDetail)
	mux.HandleFunc("GET /api/v1/customers", h.listCustomers)
	mux.HandleFunc("GET /api/v1/customers/{id}", h.getCustomer)
	mux.HandleFunc("PATCH /api/v1/customers/{id}", h.updateCustomer)
	mux.HandleFunc("GET /api/v1/customers/{id}/prescriptions", h.listCustomerPrescriptions)
	mux.HandleFunc("POST /api/v1/customers/{id}/prescriptions", h.createPrescription)
	mux.HandleFunc("GET /api/v1/customers/{id}/refills", h.listCustomerRefills)
	mux.HandleFunc("POST /api/v1/customers/{id}/refills", h.createRefill)
	mux.HandleFunc("PATCH /api/v1/refills/{id}", h.updateRefillStatus)
	mux.HandleFunc("POST /api/v1/carts", h.createCart)
	mux.HandleFunc("GET /api/v1/carts/{id}", h.getCartDetail)
	mux.HandleFunc("POST /api/v1/carts/{id}/items", h.addCartItem)
	mux.HandleFunc("DELETE /api/v1/carts/{id}/items/{medication_id}", h.removeCartItem)
	mux.HandleFunc("POST /api/v1/carts/{id}/checkout", h.checkoutCart)
	mux.HandleFunc("GET /api/v1/customers/{id}/orders", h.listCustomerOrders)
	mux.HandleFunc("GET /api/v1/orders/{id}", h.getOrderDetail)
}

func (h *Handlers) listBranches(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	items, err := h.pharmacy.ListBranches(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) getBranch(w http.ResponseWriter, r *http.Request) {
	item, err := h.pharmacy.GetBranch(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handlers) listBranchInventory(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	items, err := h.pharmacy.ListBranchInventory(r.Context(), r.PathValue("id"), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) searchMedications(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	q := r.URL.Query().Get("q")
	items, err := h.pharmacy.SearchMedications(r.Context(), q, p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) getMedicationDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := h.pharmacy.GetMedicationDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *Handlers) listCustomers(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	items, err := h.pharmacy.ListCustomers(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) getCustomer(w http.ResponseWriter, r *http.Request) {
	item, err := h.pharmacy.GetCustomer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handlers) updateCustomer(w http.ResponseWriter, r *http.Request) {
	var in services.UpdateCustomerInput
	if !h.decode(w, r, &in) {
		return
	}
	item, err := h.pharmacy.UpdateCustomer(r.Context(), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handlers) listCustomerPrescriptions(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	items, err := h.pharmacy.ListCustomerPrescriptions(r.Context(), r.PathValue("id"), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) createPrescription(w http.ResponseWriter, r *http.Request) {
	var in services.CreatePrescriptionInput
	if !h.decode(w, r, &in) {
		return
	}
	item, err := h.pharmacy.CreatePrescription(r.Context(), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, item)
}

func (h *Handlers) listCustomerRefills(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	items, err := h.pharmacy.ListCustomerRefills(r.Context(), r.PathValue("id"), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) createRefill(w http.ResponseWriter, r *http.Request) {
	var in services.CreateRefillInput
	if !h.decode(w, r, &in) {
		return
	}
	item, err := h.pharmacy.CreateRefill(r.Context(), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, item)
}

func (h *Handlers) updateRefillStatus(w http.ResponseWriter, r *http.Request) {
	var in services.UpdateRefillStatusInput
	if !h.decode(w, r, &in) {
		return
	}
	item, err := h.pharmacy.UpdateRefillStatus(r.Context(), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handlers) createCart(w http.ResponseWriter, r *http.Request) {
	var in services.CreateCartInput
	if !h.decode(w, r, &in) {
		return
	}
	detail, err := h.pharmacy.CreateOrGetActiveCart(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, detail)
}

func (h *Handlers) getCartDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := h.pharmacy.GetCartDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *Handlers) addCartItem(w http.ResponseWriter, r *http.Request) {
	var in services.AddCartItemInput
	if !h.decode(w, r, &in) {
		return
	}
	detail, err := h.pharmacy.AddCartItem(r.Context(), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *Handlers) removeCartItem(w http.ResponseWriter, r *http.Request) {
	detail, err := h.pharmacy.RemoveCartItem(r.Context(), r.PathValue("id"), r.PathValue("medication_id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *Handlers) checkoutCart(w http.ResponseWriter, r *http.Request) {
	var in services.CheckoutCartInput
	if !h.decode(w, r, &in) {
		return
	}
	detail, err := h.pharmacy.CheckoutCart(r.Context(), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, detail)
}

func (h *Handlers) listCustomerOrders(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	items, err := h.pharmacy.ListCustomerOrders(r.Context(), r.PathValue("id"), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) getOrderDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := h.pharmacy.GetOrderDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

import { apiGet, apiPost, apiPut, apiPatch, type PageResponse } from "@/lib/api";

export interface Customer {
  id: string;
  customerId: string;
  firstName: string;
  lastName: string;
  email?: string;
  phone?: string;
  dateOfBirth?: string;
  nationalId?: string;
  gender?: string;
  address?: string;
  customerType: string;
  status: string;
  kycStatus?: string;
  source?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface CreateCustomerRequest {
  customerId: string;
  firstName: string;
  lastName: string;
  email?: string;
  phone?: string;
  dateOfBirth?: string;
  nationalId?: string;
  gender?: string;
  address?: string;
  customerType?: string;
  source?: string;
}

export interface UpdateCustomerRequest {
  firstName?: string;
  lastName?: string;
  email?: string;
  phone?: string;
  dateOfBirth?: string;
  nationalId?: string;
  gender?: string;
  address?: string;
  customerType?: string;
}

const BASE = "/proxy/auth/api/v1/customers";

export const customerService = {
  async listCustomers(page = 0, size = 20): Promise<PageResponse<Customer>> {
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    const result = await apiGet<PageResponse<Customer>>(`${BASE}?${params}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to list customers");
    }
    return result.data;
  },

  async getCustomer(id: string): Promise<Customer> {
    const result = await apiGet<Customer>(`${BASE}/${id}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch customer");
    }
    return result.data;
  },

  async createCustomer(req: CreateCustomerRequest): Promise<Customer> {
    const result = await apiPost<Customer>(BASE, req);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to create customer");
    }
    return result.data;
  },

  async updateCustomer(id: string, req: UpdateCustomerRequest): Promise<Customer> {
    const result = await apiPut<Customer>(`${BASE}/${id}`, req);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to update customer");
    }
    return result.data;
  },

  async updateStatus(id: string, status: string): Promise<Customer> {
    const result = await apiPatch<Customer>(`${BASE}/${id}/status`, { status });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to update customer status");
    }
    return result.data;
  },

  async updateKyc(id: string, kycStatus: string): Promise<Customer> {
    const result = await apiPatch<Customer>(`${BASE}/${id}/kyc`, { kycStatus });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to update customer KYC status");
    }
    return result.data;
  },

  async searchCustomers(q: string): Promise<Customer[]> {
    const result = await apiGet<Customer[]>(`${BASE}/search?q=${encodeURIComponent(q)}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to search customers");
    }
    return result.data;
  },
};

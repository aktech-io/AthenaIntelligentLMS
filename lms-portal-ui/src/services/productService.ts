import { apiGet, apiPost, apiPut, type PageResponse } from "@/lib/api";

export interface Product {
  id: string;
  name: string;
  productCode: string;
  productType: string;
  currency: string;
  status: string;
  nominalRate: number;
  minAmount: number;
  maxAmount: number;
  minTenorDays: number;
  maxTenorDays: number;
  gracePeriodDays: number;
  repaymentFrequency: string;
  scheduleType: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface CreateProductRequest {
  name: string;
  productCode: string;
  productType: string;
  currency: string;
  nominalRate: number;
  minAmount: number;
  maxAmount: number;
  minTenorDays: number;
  maxTenorDays: number;
  gracePeriodDays: number;
  repaymentFrequency: string;
  scheduleType: string;
  penaltyRate?: number;
  penaltyGraceDays?: number;
  requiresTwoPersonAuth?: boolean;
  authThresholdAmount?: number;
}

export type UpdateProductRequest = Partial<CreateProductRequest>;

const BASE = "/proxy/products/api/v1/products";

export const productService = {
  async listProducts(
    page = 0,
    size = 20,
    status?: string
  ): Promise<PageResponse<Product>> {
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    if (status) params.set("status", status);
    const result = await apiGet<PageResponse<Product>>(`${BASE}?${params}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to list products");
    }
    return result.data;
  },

  async getProduct(id: string): Promise<Product> {
    const result = await apiGet<Product>(`${BASE}/${id}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch product");
    }
    return result.data;
  },

  async createProduct(req: CreateProductRequest): Promise<Product> {
    const result = await apiPost<Product>(BASE, req);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to create product");
    }
    return result.data;
  },

  async updateProduct(id: string, req: UpdateProductRequest): Promise<Product> {
    const result = await apiPut<Product>(`${BASE}/${id}`, req);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to update product");
    }
    return result.data;
  },

  async activateProduct(id: string): Promise<Product> {
    const result = await apiPost<Product>(`${BASE}/${id}/activate`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to activate product");
    }
    return result.data;
  },

  async deactivateProduct(id: string): Promise<Product> {
    const result = await apiPost<Product>(`${BASE}/${id}/deactivate`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to deactivate product");
    }
    return result.data;
  },
};

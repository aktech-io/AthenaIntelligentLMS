import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { formatKES } from "@/lib/format";
import {
  Search, Plus, Settings2, Copy, BarChart3,
} from "lucide-react";
import { motion } from "framer-motion";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { productService, type Product } from "@/services/productService";

const statusColors: Record<string, string> = {
  active: "bg-success/15 text-success border-success/30",
  ACTIVE: "bg-success/15 text-success border-success/30",
  paused: "bg-warning/15 text-warning border-warning/30",
  PAUSED: "bg-warning/15 text-warning border-warning/30",
  discontinued: "bg-muted text-muted-foreground border-border",
  DISCONTINUED: "bg-muted text-muted-foreground border-border",
  DRAFT: "bg-info/15 text-info border-info/30",
};

const typeColors: Record<string, string> = {
  Loan: "bg-info/15 text-info border-info/30",
  LOAN: "bg-info/15 text-info border-info/30",
  Savings: "bg-success/15 text-success border-success/30",
  SAVINGS: "bg-success/15 text-success border-success/30",
  Wallet: "bg-accent/15 text-accent-foreground border-accent/30",
  WALLET: "bg-accent/15 text-accent-foreground border-accent/30",
  Float: "bg-[hsl(42,56%,54%)]/15 text-[hsl(42,56%,54%)] border-[hsl(42,56%,54%)]/30",
  FLOAT: "bg-[hsl(42,56%,54%)]/15 text-[hsl(42,56%,54%)] border-[hsl(42,56%,54%)]/30",
};

/** Adapter: map a backend Product to the shape the card expects */
function adaptProduct(p: Product) {
  return {
    id: p.id,
    name: p.name,
    code: p.productCode,
    type: p.productType,
    status: p.status?.toLowerCase() ?? "active",
    icon: "📦",
    activeAccounts: 0,
    portfolioValue: 0,
    annualRate: p.nominalRate,
    repaymentFrequency: p.repaymentFrequency ?? "—",
  };
}

const ProductsPage = () => {
  const [search, setSearch] = useState("");
  const navigate = useNavigate();

  const { data: apiPage, isLoading } = useQuery({
    queryKey: ["products", "list"],
    queryFn: () => productService.listProducts(0, 100),
    staleTime: 60_000,
    retry: false,
  });

  const rawProducts =
    apiPage && apiPage.content.length > 0
      ? apiPage.content.map(adaptProduct)
      : [];

  const filtered = rawProducts.filter(
    (p) =>
      !search ||
      p.name.toLowerCase().includes(search.toLowerCase()) ||
      p.code.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <DashboardLayout
      title="Product Catalogue"
      subtitle="All active lending, savings, and wallet products"
      breadcrumbs={[
        { label: "Home", href: "/" },
        { label: "Products" },
        { label: "Product Catalogue" },
      ]}
    >
      <div className="space-y-4">
        {/* Action bar */}
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div className="relative w-full sm:w-72">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              placeholder="Search products..."
              className="pl-8 h-9 text-xs font-sans"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <Button
            size="sm"
            className="text-xs font-sans bg-primary hover:bg-primary/90"
            onClick={() => navigate("/product-config")}
          >
            <Plus className="mr-1.5 h-3.5 w-3.5" /> New Product
          </Button>
        </div>

        {/* Loading skeleton */}
        {isLoading && (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {Array.from({ length: 8 }).map((_, i) => (
              <Card key={i}>
                <CardContent className="p-4 space-y-3">
                  <Skeleton className="h-6 w-3/4" />
                  <Skeleton className="h-4 w-1/2" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-8 w-full" />
                </CardContent>
              </Card>
            ))}
          </div>
        )}

        {/* Empty state */}
        {!isLoading && filtered.length === 0 && (
          <div className="text-muted-foreground p-8 text-center">
            {search ? "No products match your search." : "No products available."}
          </div>
        )}

        {/* Product grid */}
        {!isLoading && filtered.length > 0 && (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {filtered.map((product, i) => (
              <motion.div
                key={product.id}
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: i * 0.05, duration: 0.3 }}
              >
                <Card className="hover:shadow-md hover:border-accent/30 transition-all cursor-pointer group">
                  <CardContent className="p-4">
                    <div className="flex items-start justify-between mb-3">
                      <div className="flex items-center gap-2.5">
                        <span className="text-2xl">{product.icon}</span>
                        <div>
                          <h3 className="text-sm font-sans font-semibold leading-tight">
                            {product.name}
                          </h3>
                          <p className="text-[11px] font-mono text-muted-foreground">
                            {product.code}
                          </p>
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 mb-3">
                      <Badge
                        variant="outline"
                        className={`text-[11px] font-sans font-semibold ${typeColors[product.type] ?? ""}`}
                      >
                        {product.type}
                      </Badge>
                      <Badge
                        variant="outline"
                        className={`text-[11px] font-sans font-semibold capitalize ${statusColors[product.status] ?? ""}`}
                      >
                        {product.status}
                      </Badge>
                    </div>

                    {(product.type === "Loan" ||
                      product.type === "LOAN" ||
                      product.type === "Float" ||
                      product.type === "FLOAT") ? (
                      <div className="grid grid-cols-2 gap-2 mb-3 pt-2 border-t">
                        <div>
                          <p className="text-[11px] text-muted-foreground font-sans">Rate</p>
                          <p className="text-xs font-mono font-semibold">
                            {product.annualRate}% p.a.
                          </p>
                        </div>
                        <div>
                          <p className="text-[11px] text-muted-foreground font-sans">Frequency</p>
                          <p className="text-xs font-sans font-medium">
                            {product.repaymentFrequency}
                          </p>
                        </div>
                      </div>
                    ) : null}

                    <div className="flex items-center gap-1.5 pt-2 border-t">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-[11px] h-7 font-sans flex-1"
                        onClick={(e) => {
                          e.stopPropagation();
                          navigate("/product-config");
                        }}
                      >
                        <Settings2 className="h-3 w-3 mr-1" /> Configure
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-[11px] h-7 font-sans flex-1"
                      >
                        <Copy className="h-3 w-3 mr-1" /> Clone
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-[11px] h-7 font-sans flex-1"
                      >
                        <BarChart3 className="h-3 w-3 mr-1" /> Analytics
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </div>
        )}
      </div>
    </DashboardLayout>
  );
};

export default ProductsPage;

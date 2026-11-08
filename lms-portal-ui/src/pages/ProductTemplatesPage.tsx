import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { productTemplates } from "@/data/productConfig";
import { useNavigate } from "react-router-dom";
import { motion } from "framer-motion";
import { ArrowRight, Package } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { productService, type Product } from "@/services/productService";
import { formatKES } from "@/lib/format";

const ProductTemplatesPage = () => {
  const navigate = useNavigate();

  const { data: apiPage, isLoading } = useQuery({
    queryKey: ["products-list"],
    queryFn: () => productService.listProducts(0, 50),
    staleTime: 60_000,
    retry: false,
  });

  const products: Product[] = apiPage?.content ?? [];

  return (
    <DashboardLayout
      title="Product Templates"
      subtitle="Pre-configured product blueprints and active products"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Products" }, { label: "Product Templates" }]}
    >
      <div className="space-y-6">
        {/* Templates Section */}
        <div>
          <h2 className="text-sm font-sans font-semibold mb-3">Blueprint Templates</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            {productTemplates.map((tpl, i) => (
              <motion.div
                key={tpl.id}
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: i * 0.06, duration: 0.3 }}
              >
                <Card className="hover:shadow-md hover:border-accent/30 transition-all h-full flex flex-col">
                  <CardContent className="p-5 flex flex-col flex-1">
                    <div className="flex items-center gap-3 mb-3">
                      <span className="text-3xl">{tpl.icon}</span>
                      <div>
                        <h3 className="text-sm font-sans font-semibold leading-tight">{tpl.name}</h3>
                        <Badge variant="outline" className="text-[11px] mt-1 font-sans">Template</Badge>
                      </div>
                    </div>
                    <p className="text-xs text-muted-foreground font-sans mb-3 flex-1">{tpl.description}</p>
                    <div className="bg-muted/50 rounded-md px-3 py-2 mb-4">
                      <p className="text-[11px] text-muted-foreground font-sans uppercase tracking-wider mb-0.5">Key Parameters</p>
                      <p className="text-xs font-mono font-medium">{tpl.keyParams}</p>
                    </div>
                    <Button size="sm" className="w-full text-xs font-sans" onClick={() => navigate("/product-config")}>
                      Use Template <ArrowRight className="ml-1.5 h-3.5 w-3.5" />
                    </Button>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </div>
        </div>

        {/* Active Products */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Package className="h-4 w-4" /> Active Products ({products.length})
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : products.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-32 text-muted-foreground">
                <p className="text-sm font-medium">No products configured</p>
                <p className="text-xs mt-1">Use a template above to create your first product.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Name</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Code</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Type</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Rate</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Min Amount</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Max Amount</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Schedule</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {products.map((p) => (
                    <TableRow key={p.id} className="table-row-hover">
                      <TableCell className="text-xs font-sans font-medium">{p.name}</TableCell>
                      <TableCell className="text-xs font-mono text-muted-foreground">{p.productCode}</TableCell>
                      <TableCell className="text-xs font-sans">{p.productType}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{p.nominalRate}%</TableCell>
                      <TableCell className="text-xs font-mono text-right">{formatKES(p.minAmount)}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{formatKES(p.maxAmount)}</TableCell>
                      <TableCell className="text-xs font-sans">{p.scheduleType}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className={`text-[11px] font-sans ${
                          p.status === "ACTIVE" ? "bg-success/15 text-success border-success/30" :
                          "bg-muted text-muted-foreground border-border"
                        }`}>
                          {p.status}
                        </Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
};

export default ProductTemplatesPage;

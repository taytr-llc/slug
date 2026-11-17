// D75 statistics-only logistic-fit accelerator. This is not a fight engine.
// Protocol: b1e3a27c6dad5956b98dd340efd313998324d656a4d23d7d04b40d3fcc3424d6.
// Build: clang++ -O2 -std=c++17 -ffp-contract=off -fno-fast-math
//        -fno-associative-math fit_accelerator.cpp -o fit_accelerator
// Usage: fit_accelerator TRAIN INDICES OUT --original
//        fit_accelerator TRAIN INDICES OUT --range FIRST COUNT
// TRAIN: D75TRN01, LE32(4000), LE32(5), then 4000 rows of five LE signed
// int32 league-tenths differences and LE32 outcome half-points (0, 1, or 2).
// INDICES: D75IDX01, LE32(1000), LE32(4000), then 1000 replica-major rows
// of 4000 LE uint16 absolute training-row indexes. All files must be exact.
// OUT is exclusively created; successful lines contain only the replica ID,
// iteration count, and five raw binary64 weights as exact hex strings. Python
// owns normalization, rounding, statistical validity and Decimal checks.

#include <algorithm>
#include <array>
#include <cerrno>
#include <cfenv>
#include <cfloat>
#include <cmath>
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <exception>
#include <fcntl.h>
#include <iomanip>
#include <limits>
#include <locale>
#include <sstream>
#include <stdexcept>
#include <string>
#include <sys/stat.h>
#include <unistd.h>
#include <vector>

#ifdef __FAST_MATH__
#error "D75 accelerator forbids fast-math"
#endif

namespace {

constexpr std::size_t kRows = 4000;
constexpr std::size_t kFeatures = 5;
constexpr std::size_t kReplicas = 1000;
constexpr int kIterations = 400;
constexpr std::size_t kHeaderBytes = 16;
constexpr std::size_t kTrainingBytes = kHeaderBytes + kRows * 24;
constexpr std::size_t kIndexBytes = kHeaderBytes + kReplicas * kRows * 2;
static_assert(sizeof(double) == 8 && std::numeric_limits<double>::is_iec559 &&
                  std::numeric_limits<double>::digits == 53,
              "D75 accelerator requires IEEE binary64");
static_assert(FLT_EVAL_METHOD == 0, "D75 accelerator forbids excess precision");

struct File {
  int fd;
  explicit File(int value) : fd(value) {}
  File(const File &) = delete;
  File &operator=(const File &) = delete;
  ~File() {
    if (fd >= 0) {
      ::close(fd);
    }
  }
  void closeChecked() {
    const int value = fd;
    fd = -1;
    if (::close(value) != 0) {
      throw std::runtime_error("file close failed");
    }
  }
};

struct TrainingRow {
  std::array<double, kFeatures> x;
  double y;
};

std::uint32_t little32(const std::uint8_t *p) {
  return std::uint32_t(p[0]) | (std::uint32_t(p[1]) << 8) |
         (std::uint32_t(p[2]) << 16) | (std::uint32_t(p[3]) << 24);
}

std::uint16_t little16(const std::uint8_t *p) {
  return std::uint16_t(std::uint16_t(p[0]) | (std::uint16_t(p[1]) << 8));
}

std::int32_t signedLittle32(const std::uint8_t *p) {
  const std::uint32_t word = little32(p);
  const std::int64_t value = (word & 0x80000000U)
                                 ? std::int64_t(word) - INT64_C(4294967296)
                                 : std::int64_t(word);
  return static_cast<std::int32_t>(value);
}

std::vector<std::uint8_t> exactFile(const char *path, std::size_t expected) {
  File file(::open(path, O_RDONLY | O_NOFOLLOW | O_CLOEXEC));
  if (file.fd < 0) {
    throw std::runtime_error("input open failed");
  }
  struct stat st {};
  if (::fstat(file.fd, &st) != 0 || !S_ISREG(st.st_mode) ||
      st.st_size != static_cast<off_t>(expected)) {
    throw std::runtime_error("input must be a regular file of the exact size");
  }
  std::vector<std::uint8_t> bytes(expected);
  std::size_t offset = 0;
  while (offset < expected) {
    const ssize_t n = ::read(file.fd, bytes.data() + offset, expected - offset);
    if (n < 0 && errno == EINTR) {
      continue;
    }
    if (n <= 0) {
      throw std::runtime_error("input read failed or truncated");
    }
    offset += static_cast<std::size_t>(n);
  }
  std::uint8_t extra;
  ssize_t n;
  do {
    n = ::read(file.fd, &extra, 1);
  } while (n < 0 && errno == EINTR);
  if (n != 0) {
    throw std::runtime_error("input has trailing bytes or read failed");
  }
  file.closeChecked();
  return bytes;
}

std::array<TrainingRow, kRows> loadTraining(const char *path) {
  const auto bytes = exactFile(path, kTrainingBytes);
  if (std::memcmp(bytes.data(), "D75TRN01", 8) != 0 ||
      little32(bytes.data() + 8) != kRows ||
      little32(bytes.data() + 12) != kFeatures) {
    throw std::runtime_error("invalid training header");
  }
  std::array<TrainingRow, kRows> rows {};
  for (std::size_t i = 0; i < kRows; ++i) {
    const auto *row = bytes.data() + kHeaderBytes + i * 24;
    for (std::size_t j = 0; j < kFeatures; ++j) {
      rows[i].x[j] = static_cast<double>(signedLittle32(row + j * 4)) / 500.0;
    }
    const std::uint32_t half = little32(row + kFeatures * 4);
    if (half > 2) {
      throw std::runtime_error("invalid training outcome");
    }
    rows[i].y = static_cast<double>(half) / 2.0;
  }
  return rows;
}

std::vector<std::uint8_t> loadIndices(const char *path) {
  auto bytes = exactFile(path, kIndexBytes);
  if (std::memcmp(bytes.data(), "D75IDX01", 8) != 0 ||
      little32(bytes.data() + 8) != kReplicas ||
      little32(bytes.data() + 12) != kRows) {
    throw std::runtime_error("invalid index header");
  }
  for (std::size_t i = kHeaderBytes; i < bytes.size(); i += 2) {
    if (little16(bytes.data() + i) >= kRows) {
      throw std::runtime_error("training row index out of range");
    }
  }
  return bytes;
}

std::size_t parseRangeNumber(const char *text) {
  if (*text == '\0') {
    throw std::runtime_error("empty range number");
  }
  std::size_t value = 0;
  for (const char *p = text; *p; ++p) {
    if (*p < '0' || *p > '9' || value > kReplicas / 10) {
      throw std::runtime_error("invalid range number");
    }
    value = value * 10 + static_cast<std::size_t>(*p - '0');
    if (value > kReplicas) {
      throw std::runtime_error("range number exceeds replica count");
    }
  }
  return value;
}

std::string hexValue(double value) {
  if (std::isnan(value)) {
    return "nan";
  }
  if (std::isinf(value)) {
    return std::signbit(value) ? "-inf" : "+inf";
  }
  std::ostringstream stream;
  stream.imbue(std::locale::classic());
  stream << std::hexfloat << value;
  if (!stream) {
    throw std::runtime_error("weight serialization failed");
  }
  return stream.str();
}

void writeLine(int fd, int replica, const std::array<double, kFeatures> &weights,
               int failedIteration = 0) {
  std::ostringstream stream;
  stream.imbue(std::locale::classic());
  stream << "{\"replica\":" << replica;
  if (failedIteration != 0) {
    stream << ",\"iteration\":" << failedIteration
           << ",\"error\":\"nonfinite_arithmetic\"";
  } else {
    stream << ",\"iterations\":" << kIterations;
  }
  stream << ",\"weights_hex\":[";
  for (std::size_t j = 0; j < kFeatures; ++j) {
    if (j != 0) {
      stream << ',';
    }
    stream << '"' << hexValue(weights[j]) << '"';
  }
  stream << "]}\n";
  if (!stream) {
    throw std::runtime_error("result serialization failed");
  }
  const std::string line = stream.str();
  std::size_t offset = 0;
  while (offset < line.size()) {
    const ssize_t n = ::write(fd, line.data() + offset, line.size() - offset);
    if (n < 0 && errno == EINTR) {
      continue;
    }
    if (n <= 0) {
      throw std::runtime_error("result write failed");
    }
    offset += static_cast<std::size_t>(n);
  }
  // No user-space buffering: each complete fit is visible before the next fit.
}

std::array<double, kFeatures>
fit(const std::array<const TrainingRow *, kRows> &rows, int replica, int out) {
  std::array<double, kFeatures> weights {};
  for (int iteration = 1; iteration <= kIterations; ++iteration) {
    auto finite = [&](double value) {
      if (!std::isfinite(value)) {
        writeLine(out, replica, weights, iteration);
        throw std::runtime_error("nonfinite fit arithmetic; diagnostic retained");
      }
    };
    std::array<double, kFeatures> gradient {};
    for (const TrainingRow *row : rows) {
      double z = 0.0;
      for (std::size_t j = 0; j < kFeatures; ++j) {
        const double product = weights[j] * row->x[j];
        finite(product);
        z = z + product;
        finite(z);
      }
      z = std::max(-30.0, std::min(30.0, z));
      const double exponential = std::exp(-z);
      const double denominator = 1.0 + exponential;
      const double prediction = 1.0 / denominator;
      const double error = prediction - row->y;
      finite(error);
      for (std::size_t j = 0; j < kFeatures; ++j) {
        const double term = error * row->x[j];
        finite(term);
        gradient[j] = gradient[j] + term;
        finite(gradient[j]);
      }
    }
    std::array<double, kFeatures> next {};
    for (std::size_t j = 0; j < kFeatures; ++j) {
      const double average = gradient[j] / static_cast<double>(kRows);
      const double step = 0.5 * average;
      next[j] = weights[j] - step;
      finite(next[j]);
    }
    weights = next;
  }
  return weights;
}

} // namespace

int main(int argc, char **argv) {
  try {
    const bool original = argc == 5 && std::strcmp(argv[4], "--original") == 0;
    const bool range = argc == 7 && std::strcmp(argv[4], "--range") == 0;
    if (!original && !range) {
      throw std::runtime_error(
          "usage: fit_accelerator TRAIN INDICES OUT --original | --range FIRST COUNT");
    }
    const std::size_t first = range ? parseRangeNumber(argv[5]) : 0;
    const std::size_t count = range ? parseRangeNumber(argv[6]) : 1;
    if (first >= kReplicas || count == 0 || count > kReplicas - first) {
      throw std::runtime_error("replica range out of bounds");
    }
    if (std::fesetround(FE_TONEAREST) != 0 || std::fegetround() != FE_TONEAREST) {
      throw std::runtime_error("cannot establish round-to-nearest arithmetic");
    }
    const auto training = loadTraining(argv[1]);
    const auto indices = loadIndices(argv[2]);
    // Validate all inputs before creating output, even for --original.
    File output(::open(argv[3], O_WRONLY | O_CREAT | O_EXCL | O_NOFOLLOW | O_CLOEXEC,
                       0600));
    if (output.fd < 0) {
      throw std::runtime_error("exclusive output creation failed");
    }
    for (std::size_t r = first; r < first + count; ++r) {
      std::array<const TrainingRow *, kRows> selected {};
      for (std::size_t i = 0; i < kRows; ++i) {
        const std::size_t index =
            original ? i : little16(indices.data() + kHeaderBytes +
                                    (r * kRows + i) * 2);
        selected[i] = &training[index];
      }
      const int replica = original ? -1 : static_cast<int>(r);
      const auto weights = fit(selected, replica, output.fd);
      writeLine(output.fd, replica, weights);
    }
    if (::fsync(output.fd) != 0) {
      throw std::runtime_error("result sync failed; partial output retained");
    }
    output.closeChecked();
    return 0;
  } catch (const std::exception &error) {
    std::fprintf(stderr, "fit_accelerator: %s\n", error.what());
    return 1;
  }
}
